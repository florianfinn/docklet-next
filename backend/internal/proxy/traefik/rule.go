// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original Traefik v3 engine.

package traefik

import (
	"fmt"
	"net"
	"regexp"
	"strconv"
	"strings"
	"unicode"
)

type matcher struct {
	name string
	args []string
}
type ruleParser struct {
	s     string
	pos   int
	calls []matcher
	depth int
}

// parseRule implements the HTTP v3 matcher grammar, retaining calls for probes.
// Backtick and Go double-quoted literals are the only argument forms in v3.
func parseRule(s string) ([]matcher, error) {
	p := &ruleParser{s: s}
	if len(s) > 16384 {
		return nil, fmt.Errorf("rule exceeds limit")
	}
	if err := p.expr(); err != nil {
		return nil, err
	}
	p.space()
	if p.pos != len(s) {
		return nil, fmt.Errorf("unexpected rule token at %d", p.pos)
	}
	return p.calls, nil
}
func (p *ruleParser) space() {
	for p.pos < len(p.s) && unicode.IsSpace(rune(p.s[p.pos])) {
		p.pos++
	}
}
func (p *ruleParser) take(s string) bool {
	p.space()
	if strings.HasPrefix(p.s[p.pos:], s) {
		p.pos += len(s)
		return true
	}
	return false
}
func (p *ruleParser) expr() error {
	if err := p.and(); err != nil {
		return err
	}
	for p.take("||") {
		if err := p.and(); err != nil {
			return err
		}
	}
	return nil
}
func (p *ruleParser) and() error {
	if err := p.atom(); err != nil {
		return err
	}
	for p.take("&&") {
		if err := p.atom(); err != nil {
			return err
		}
	}
	return nil
}
func (p *ruleParser) atom() error {
	p.depth++
	defer func() { p.depth-- }()
	if p.depth > 128 {
		return fmt.Errorf("rule nesting exceeds limit")
	}
	if p.take("!") {
		return p.atom()
	}
	if p.take("(") {
		if err := p.expr(); err != nil {
			return err
		}
		if !p.take(")") {
			return fmt.Errorf("missing closing parenthesis")
		}
		return nil
	}
	p.space()
	start := p.pos
	for p.pos < len(p.s) && ((p.s[p.pos] >= 'A' && p.s[p.pos] <= 'Z') || (p.s[p.pos] >= 'a' && p.s[p.pos] <= 'z')) {
		p.pos++
	}
	name := p.s[start:p.pos]
	if !p.take("(") {
		return fmt.Errorf("expected matcher")
	}
	var args []string
	for {
		p.space()
		start = p.pos
		if p.pos >= len(p.s) || (p.s[p.pos] != '`' && p.s[p.pos] != '"') {
			return fmt.Errorf("expected string argument")
		}
		quote := p.s[p.pos]
		p.pos++
		for p.pos < len(p.s) && p.s[p.pos] != quote {
			if quote == '"' && p.s[p.pos] == '\\' {
				p.pos++
			}
			p.pos++
		}
		if p.pos >= len(p.s) {
			return fmt.Errorf("unterminated matcher string")
		}
		p.pos++
		v, err := strconv.Unquote(p.s[start:p.pos])
		if err != nil {
			return err
		}
		args = append(args, v)
		if p.take(")") {
			break
		}
		if !p.take(",") {
			return fmt.Errorf("expected comma or parenthesis")
		}
	}
	name = canonicalMatcher(name)
	if err := checkMatcher(name, args); err != nil {
		return err
	}
	p.calls = append(p.calls, matcher{name, args})
	return nil
}

var matchers = []string{"Host", "HostRegexp", "Path", "PathPrefix", "PathRegexp", "Method", "ClientIP", "Header", "HeaderRegexp", "Query", "QueryRegexp"}

// canonicalMatcher accepts the spellings Traefik v3.7.16 registers per matcher:
// exact, lower, upper and title case (rules.NewParser).
func canonicalMatcher(name string) string {
	for _, m := range matchers {
		lower := strings.ToLower(m)
		if name == m || name == lower || name == strings.ToUpper(m) || name == strings.ToUpper(lower[:1])+lower[1:] {
			return m
		}
	}
	return name
}

// canonicalRule spells a rule's leading matcher canonically for literal comparisons.
func canonicalRule(rule string) string {
	rule = strings.TrimSpace(rule)
	if i := strings.IndexByte(rule, '('); i > 0 {
		return canonicalMatcher(rule[:i]) + rule[i:]
	}
	return rule
}
func checkMatcher(name string, a []string) error {
	n := 1
	switch name {
	case "Header", "HeaderRegexp":
		n = 2
	case "Query", "QueryRegexp":
		if len(a) == 1 {
			n = 1
		} else {
			n = 2
		}
	case "Host", "HostRegexp", "Path", "PathPrefix", "PathRegexp", "Method", "ClientIP":
	default:
		return fmt.Errorf("unknown v3 matcher %q", name)
	}
	if len(a) != n {
		return fmt.Errorf("%s expects %d arguments", name, n)
	}
	for _, s := range a {
		if s == "" {
			return fmt.Errorf("empty args for matcher %s", name)
		}
		if strings.ContainsAny(s, "\r\n\x00") {
			return fmt.Errorf("control character in matcher")
		}
	}
	// Matcher arities and ASCII requirements follow Traefik v3.7.16's HTTP muxer.
	if name == "Host" || name == "HostRegexp" {
		for _, r := range a[0] {
			if r > 127 {
				return fmt.Errorf("non-ASCII host matcher")
			}
		}
	}
	switch name {
	case "Path", "PathPrefix":
		if !strings.HasPrefix(a[0], "/") {
			return fmt.Errorf("path must start with slash")
		}
	case "HostRegexp", "PathRegexp":
		if regexp.MustCompile(`\{[A-Za-z][^}]*:`).MatchString(a[0]) {
			return fmt.Errorf("v2 named regexp syntax")
		}
		if _, err := regexp.Compile(a[0]); err != nil {
			return err
		}
	case "HeaderRegexp", "QueryRegexp":
		if len(a) == 2 {
			if _, err := regexp.Compile(a[1]); err != nil {
				return err
			}
		}
	case "ClientIP":
		if _, _, err := net.ParseCIDR(a[0]); err != nil {
			if net.ParseIP(a[0]) == nil {
				return err
			}
		}

	}
	return nil
}
