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
	"reflect"
	"sort"
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
func (d *document) replace(n *yaml.Node, v any, edits *[]edit) error {
	if reflect.DeepEqual(value(n), v) {
		return nil
	}
	if m, ok := v.(map[string]any); ok && n.Kind == yaml.MappingNode && len(m) > 0 && len(n.Content) > 0 {
		return d.patch(n, m, edits)
	}
	if a, ok := v.([]any); ok && n.Kind == yaml.SequenceNode && len(a) == len(n.Content) {
		for i, c := range n.Content {
			if err := d.replace(c, a[i], edits); err != nil {
				return err
			}
		}
		return nil
	}
	if a, ok := v.([]string); ok && n.Kind == yaml.SequenceNode && len(a) == len(n.Content) {
		for i, c := range n.Content {
			if err := d.replace(c, a[i], edits); err != nil {
				return err
			}
		}
		return nil
	}
	if n.Kind == yaml.SequenceNode && n.Style&yaml.FlowStyle == 0 && len(n.Content) > 0 {
		var items []any
		switch a := v.(type) {
		case []any:
			items = a
		case []string:
			for _, x := range a {
				items = append(items, x)
			}
		}
		if items != nil && len(items) > 0 {
			for j, c := range n.Content {
				if j < len(items) {
					if err := d.replace(c, items[j], edits); err != nil {
						return err
					}
				} else {
					sp := span{lineStart(d.text, d.spans[c].start), lineEnd(d.text, d.spans[c].end)}
					*edits = append(*edits, edit{sp, comments(d.text[sp.start:sp.end], n.Column-1)})
				}
			}
			if len(items) > len(n.Content) {
				text, err := render(items[len(n.Content):], 0)
				if err != nil {
					return err
				}
				indent := strings.Repeat(" ", n.Column-1)
				text = indent + strings.ReplaceAll(text, "\n", "\n"+indent) + "\n"
				pos := lineEnd(d.text, d.spans[n].end)
				if pos > 0 && d.text[pos-1] != '\n' {
					text = "\n" + text
				}
				*edits = append(*edits, edit{span{pos, pos}, text})
			}
			return nil
		}
	}
	style := n.Style
	switch v.(type) {
	case map[string]any, []string, []any:
		style = yaml.FlowStyle
	}
	text, err := render(v, style)
	if err != nil {
		return err
	}
	sp := d.spans[n]
	if n.Kind != yaml.ScalarNode {
		if n.Style&yaml.FlowStyle == 0 {
			sp.end = lineEnd(d.text, sp.end)
			text += "\n" + comments(d.text[sp.start:sp.end], n.Column-1)
		} else if retained := comments(d.text[sp.start:sp.end], n.Column-1); retained != "" {
			text += "\n" + retained + strings.Repeat(" ", n.Column-1)
		}
	}
	*edits = append(*edits, edit{sp, text})
	return nil
}
func comments(s string, indent int) string {
	out := ""
	for _, line := range strings.Split(s, "\n") {
		q := byte(0)
		for i := 0; i < len(line); i++ {
			c := line[i]
			if q != 0 {
				if c == '\\' && q == '"' {
					i++
					continue
				}
				if c == q {
					if q == '\'' && i+1 < len(line) && line[i+1] == '\'' {
						i++
						continue
					}
					q = 0
				}
				continue
			}
			if c == '\'' || c == '"' {
				q = c
			}
			if c == '#' && (i == 0 || line[i-1] == ' ') {
				out += strings.Repeat(" ", indent) + line[i:] + "\n"
				break
			}
		}
	}
	return out
}
func (d *document) remove(m *yaml.Node, i int) (edit, error) {
	if m.Style&yaml.FlowStyle != 0 {
		return edit{}, fmt.Errorf("flow-map removal unsupported")
	}
	k, v := m.Content[i], m.Content[i+1]
	start := lineStart(d.text, d.spans[k].start)
	end := lineEnd(d.text, d.spans[v].end)
	text := comments(d.text[start:end], k.Column-1)
	if len(m.Content) == 2 {
		text += strings.Repeat(" ", k.Column-1) + "{}\n"
	}
	return edit{span{start, end}, text}, nil
}
func (d *document) patch(n *yaml.Node, m map[string]any, edits *[]edit) error {
	if n == nil || n.Kind != yaml.MappingNode {
		return fmt.Errorf("expected map")
	}
	additions := map[string]any{}
	for k, v := range m {
		c := field(n, k)
		if c == nil {
			if v != nil {
				additions[k] = v
			}
			continue
		}
		if v == nil {
			for i := 0; i < len(n.Content); i += 2 {
				if n.Content[i].Value == k {
					e, err := d.remove(n, i)
					if err != nil {
						return err
					}
					*edits = append(*edits, e)
				}
			}
		} else if err := d.replace(c, v, edits); err != nil {
			return err
		}
	}
	if len(additions) > 0 {
		if len(n.Content) == 0 {
			return d.replace(n, additions, edits)
		}
		text, err := render(additions, 0)
		if err != nil {
			return err
		}
		pos := lineEnd(d.text, d.spans[n].end)
		indent := n.Content[0].Column - 1
		if n.Style&yaml.FlowStyle != 0 {
			text, err = render(additions, yaml.FlowStyle)
			if err != nil {
				return err
			}
			text = ", " + strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(text, "{"), "}"))
			pos = d.spans[n].end - 1
		} else {
			text = strings.Repeat(" ", indent) + strings.ReplaceAll(text, "\n", "\n"+strings.Repeat(" ", indent)) + "\n"
			if pos > 0 && d.text[pos-1] != '\n' {
				text = "\n" + text
			}
		}
		*edits = append(*edits, edit{span{pos, pos}, text})
	}
	return nil
}
func (d *document) apply(edits []edit) (string, error) {
	if strings.Contains(d.text, "\r") {
		return "", fmt.Errorf("CRLF unsupported")
	}
	sort.SliceStable(edits, func(i, j int) bool {
		return edits[i].start > edits[j].start || edits[i].start == edits[j].start && insertionIndent(edits[i].text) < insertionIndent(edits[j].text)
	})
	text := d.text
	boundary := len(text)
	for _, e := range edits {
		if e.start < 0 || e.end > boundary || e.end < e.start {
			return "", fmt.Errorf("overlapping edits")
		}
		text = text[:e.start] + e.text + text[e.end:]
		boundary = e.start
	}
	if _, err := parseDocument(text, ""); err != nil {
		return "", err
	}
	return text, nil
}

func insertionIndent(s string) int {
	s = strings.TrimLeft(s, "\n")
	return len(s) - len(strings.TrimLeft(s, " "))
}
