// Source: docklet-next/backend/internal/proxy/traefik/yaml.go
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Moved span editing implementation and exported a resolver editing port.

package traefik

import (
	"fmt"
	"go.yaml.in/yaml/v4"
	"reflect"
	"sort"
	"strings"
)

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

// Splice changes only the named mapping leaves or sequence items. All other
// bytes, including comments and scalar spellings, belong to the operator.
func Splice(text string, keys []string, changes map[string]any) (string, error) {
	d, err := parseDocument(text, "")
	if err != nil {
		return "", err
	}
	n := at(d.root, keys...)
	var edits []edit
	if err = d.patch(n, changes, &edits); err != nil {
		return "", err
	}
	return d.apply(edits)
}
func (*engine) ResolverSplice(text string, keys []string, changes map[string]any) (string, error) {
	return Splice(text, keys, changes)
}
