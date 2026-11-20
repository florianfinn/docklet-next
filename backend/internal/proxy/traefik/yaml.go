// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	dto "github.com/florianfinn/docklet-next/types/proxy"
	"go.yaml.in/yaml/v4"
)

type sourceError struct {
	line  int
	cause error
}

func (e sourceError) Error() string { return e.cause.Error() }
func (e sourceError) Unwrap() error { return e.cause }
func yamlLine(err error) int {
	var load *yaml.LoadError
	if errors.As(err, &load) {
		return max(1, load.Mark.Line)
	}
	var source sourceError
	if errors.As(err, &source) {
		return max(1, source.line)
	}
	return 1
}

type document struct {
	text  string
	root  *yaml.Node
	spans map[*yaml.Node]span
}
type span struct{ start, end int }
type edit struct {
	span
	text string
}

func problem(code dto.Reason, p string, line int, detail string) error {
	return &dto.Problem{Status: 422, Code: code, Path: p, Line: line, Detail: detail}
}
func parseDocument(text, p string) (*document, error) {
	if strings.Contains(text, "\r") || strings.EqualFold(path.Ext(p), ".toml") {
		return nil, problem(dto.UnsupportedFormat, p, 1, "unsupported source format")
	}
	if strings.Contains(text, "{{") {
		return nil, problem(dto.TemplateInput, p, 1, "template input")
	}
	var root yaml.Node
	dec := yaml.NewDecoder(strings.NewReader(text))
	err := dec.Decode(&root)
	if err == io.EOF {
		// Traefik loads an empty or comment-only file as an empty configuration.
		return &document{text: text, root: &yaml.Node{Kind: yaml.MappingNode, Tag: "!!map"}, spans: map[*yaml.Node]span{}}, nil
	}
	if err != nil {
		return nil, problem(dto.InvalidGraph, p, yamlLine(err), err.Error())
	}
	var extra yaml.Node
	if err := dec.Decode(&extra); err != io.EOF {
		return nil, problem(dto.InvalidGraph, p, 1, "expected one YAML document")
	}
	if len(root.Content) != 1 || root.Content[0].Kind != yaml.MappingNode {
		return nil, problem(dto.InvalidGraph, p, 1, "expected mapping")
	}
	d := &document{text: text, root: root.Content[0], spans: map[*yaml.Node]span{}}
	if err := d.walk(d.root, false); err != nil {
		return nil, problem(dto.UnsupportedFormat, p, yamlLine(err), err.Error())
	}
	return d, nil
}
func (d *document) offset(n *yaml.Node) int {
	start := 0
	for i := 1; i < n.Line; i++ {
		k := strings.IndexByte(d.text[start:], '\n')
		if k < 0 {
			return len(d.text)
		}
		start += k + 1
	}
	for i := 1; i < n.Column && start < len(d.text); i++ {
		_, size := utf8.DecodeRuneInString(d.text[start:])
		start += size
	}
	return start
}
func (d *document) walk(n *yaml.Node, flow bool) error {
	if n.Anchor != "" || n.Kind == yaml.AliasNode || n.Style&yaml.TaggedStyle != 0 {
		return sourceError{n.Line, fmt.Errorf("YAML indirection or explicit tag")}
	}
	if n.Kind == yaml.MappingNode {
		seen := map[string]bool{}
		for i := 0; i < len(n.Content); i += 2 {
			k := n.Content[i]
			if k.Kind != yaml.ScalarNode || k.Tag != "!!str" || seen[k.Value] {
				return sourceError{k.Line, fmt.Errorf("duplicate or non-string mapping key")}
			}
			seen[k.Value] = true
		}
	}
	start := d.offset(n)
	end := start
	isFlow := n.Style&yaml.FlowStyle != 0
	for _, c := range n.Content {
		if err := d.walk(c, flow || isFlow); err != nil {
			return err
		}
		end = max(end, d.spans[c].end)
	}
	if n.Kind == yaml.ScalarNode || isFlow {
		var err error
		if isFlow {
			end, err = flowEnd(d.text, start, end, n.Kind)
		} else {
			end, err = tokenEnd(d.text, start, flow)
		}
		if err != nil {
			return sourceError{n.Line, err}
		}
	}
	if n.Kind == yaml.ScalarNode && start < end {
		var token yaml.Node
		if err := yaml.Unmarshal([]byte(d.text[start:end]), &token); err != nil || len(token.Content) != 1 || token.Content[0].Value != n.Value {
			return sourceError{n.Line, fmt.Errorf("scalar source span does not match parsed value")}
		}
	}
	d.spans[n] = span{start, end}
	return nil
}

// tokenEnd scans only the token identified by yaml/v4, never searches for a value.
func tokenEnd(s string, start int, flow bool) (int, error) {
	if start >= len(s) {
		return start, nil
	}
	quote := byte(0)
	for i := start; i < len(s); i++ {
		c := s[i]
		if quote != 0 {
			if c == '\\' && quote == '"' {
				i++
				continue
			}
			if c == quote {
				if quote == '\'' && i+1 < len(s) && s[i+1] == '\'' {
					i++
					continue
				}
				return i + 1, nil
			}
			continue
		}
		if i == start && (c == '"' || c == '\'') {
			quote = c
			continue
		}
		if (c == ':' && (i+1 == len(s) || strings.ContainsRune(" \t\n{}[]", rune(s[i+1])))) || c == '\n' || (c == '#' && (i == start || s[i-1] == ' ')) || (flow && strings.ContainsRune(",]}", rune(c))) {
			return len(strings.TrimRight(s[start:i], " \t")) + start, nil
		}
		if i == start && (c == '|' || c == '>') {
			return 0, fmt.Errorf("multiline scalar edit unsupported")
		}
	}
	if quote != 0 {
		return 0, fmt.Errorf("unterminated source token")
	}
	return len(strings.TrimRight(s[start:], " \t\n")) + start, nil
}

// Child spans come from parsed YAML nodes. Starting after the last child avoids
// treating quotes embedded in a plain scalar as flow-collection delimiters.
func flowEnd(s string, start, end int, kind yaml.Kind) (int, error) {
	close := byte('}')
	if kind == yaml.SequenceNode {
		close = ']'
	}
	for i := max(start+1, end); i < len(s); i++ {
		switch s[i] {
		case ' ', '\t', '\r', '\n', ',':
			continue
		case '#':
			i = lineEnd(s, i) - 1
		default:
			if s[i] == close {
				return i + 1, nil
			}
			return 0, fmt.Errorf("unexpected flow source token")
		}
	}
	return 0, fmt.Errorf("unterminated flow source span")
}
func field(n *yaml.Node, key string) *yaml.Node {
	if n == nil || n.Kind != yaml.MappingNode {
		return nil
	}
	for i := 0; i < len(n.Content); i += 2 {
		if n.Content[i].Value == key {
			return n.Content[i+1]
		}
	}
	return nil
}
func at(n *yaml.Node, keys ...string) *yaml.Node {
	for _, k := range keys {
		n = field(n, k)
	}
	return n
}
func value(n *yaml.Node) any {
	if n == nil {
		return nil
	}
	var v any
	_ = n.Decode(&v)
	return v
}
func str(n *yaml.Node) string {
	if n != nil && n.Tag == "!!str" {
		return n.Value
	}
	return ""
}
func list(n *yaml.Node) []string {
	out := []string{}
	if n != nil && n.Kind == yaml.SequenceNode {
		for _, v := range n.Content {
			out = append(out, str(v))
		}
	}
	return out
}
func keys(n *yaml.Node) []string {
	out := []string{}
	if n != nil && n.Kind == yaml.MappingNode {
		for i := 0; i < len(n.Content); i += 2 {
			out = append(out, n.Content[i].Value)
		}
	}
	return out
}
func lineStart(s string, p int) int { return strings.LastIndex(s[:p], "\n") + 1 }
func lineEnd(s string, p int) int {
	if k := strings.IndexByte(s[p:], '\n'); k >= 0 {
		return p + k + 1
	}
	return len(s)
}
func render(v any, style yaml.Style) (string, error) {
	var n yaml.Node
	if err := n.Encode(v); err != nil {
		return "", err
	}
	n.Style = style
	var b bytes.Buffer
	enc := yaml.NewEncoder(&b)
	enc.SetIndent(2)
	if err := enc.Encode(&n); err != nil {
		return "", err
	}
	return strings.TrimSuffix(b.String(), "\n"), nil
}
