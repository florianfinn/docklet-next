// Source: docklet-next/issues/327
// Revision: dd3ce68e369f98ba49879ac2f5ed38311a52e45e
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original guard that holds every helper shell script to the shell features of the pinned tools image.

package bindpath

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Helper scripts run in BusyBox ash built from deploy/tools/busybox.config. Host shells used by unit tests accept
// more, so each construct the config leaves out is refused here by its feature flag.
var shellFeatures = []struct {
	flag    string
	pattern *regexp.Regexp
	what    string
}{
	{"CONFIG_FEATURE_SH_MATH", regexp.MustCompile(`\$\(\(`), "arithmetic expansion"},
	{"CONFIG_ASH_BASH_COMPAT", regexp.MustCompile(`\[\[|\$'|<<<|\$\{[A-Za-z_][A-Za-z0-9_]*(:[-+=?]?[0-9]|//?)`), "bash extension"},
	{"CONFIG_ASH_ECHO", regexp.MustCompile(`(^|[\s;&|(])echo(\s|$)`), "echo builtin"},
	{"CONFIG_TEST1", regexp.MustCompile(`(^|[\s;&|(!])\[\s`), "[ applet"},
}

// scripts returns every literal passed as `sh -c <script>` in non-test backend code, resolving constants across
// packages; an argument it cannot resolve fails the test, so the set stays closed.
func scripts(t *testing.T, root string) map[string]string {
	t.Helper()
	fset := token.NewFileSet()
	type named struct {
		expr ast.Expr
		pkg  string
	}
	consts := map[string]named{}
	files := map[string]*ast.File{}
	require.NoError(t, filepath.WalkDir(root, func(file string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(file, ".go") || strings.HasSuffix(file, "_test.go") {
			return err
		}
		parsed, err := parser.ParseFile(fset, file, nil, 0)
		if err != nil {
			return err
		}
		files[file] = parsed
		for _, decl := range parsed.Decls {
			if gen, ok := decl.(*ast.GenDecl); ok && (gen.Tok == token.CONST || gen.Tok == token.VAR) {
				for _, spec := range gen.Specs {
					value := spec.(*ast.ValueSpec)
					for i, name := range value.Names {
						if i < len(value.Values) {
							consts[parsed.Name.Name+"."+name.Name] = named{value.Values[i], parsed.Name.Name}
						}
					}
				}
			}
		}
		return nil
	}))
	var resolve func(pkg string) func(string) (string, bool)
	resolve = func(pkg string) func(string) (string, bool) {
		return func(name string) (string, bool) {
			if !strings.Contains(name, ".") {
				name = pkg + "." + name
			}
			c, ok := consts[name]
			if !ok {
				return "", false
			}
			return literal(c.expr, resolve(c.pkg))
		}
	}
	found := map[string]string{}
	for _, parsed := range files {
		ast.Inspect(parsed, func(n ast.Node) bool {
			list, ok := n.(*ast.CompositeLit)
			if !ok {
				return true
			}
			for i := 0; i+2 < len(list.Elts); i++ {
				first, _ := literal(list.Elts[i], nil)
				second, _ := literal(list.Elts[i+1], nil)
				if first != "sh" || second != "-c" {
					continue
				}
				text, ok := literal(list.Elts[i+2], resolve(parsed.Name.Name))
				where := fset.Position(list.Elts[i+2].Pos()).String()
				require.Truef(t, ok, "%s: helper script is not a resolvable string constant", where)
				found[where] = text
			}
			return true
		})
	}
	return found
}

func literal(expr ast.Expr, lookup func(string) (string, bool)) (string, bool) {
	switch e := expr.(type) {
	case *ast.BasicLit:
		if e.Kind != token.STRING {
			return "", false
		}
		v, err := strconv.Unquote(e.Value)
		return v, err == nil
	case *ast.BinaryExpr:
		left, ok := literal(e.X, lookup)
		right, ok2 := literal(e.Y, lookup)
		return left + right, ok && ok2 && e.Op == token.ADD
	case *ast.Ident:
		if lookup != nil {
			return lookup(e.Name)
		}
	case *ast.SelectorExpr:
		if pkg, ok := e.X.(*ast.Ident); ok && lookup != nil {
			return lookup(pkg.Name + "." + e.Sel.Name)
		}
	}
	return "", false
}

func TestHelperScriptsUseOnlyToolsShellFeatures(t *testing.T) {
	config, err := os.ReadFile("../../../../deploy/tools/busybox.config")
	require.NoError(t, err)
	found := scripts(t, "../../..")
	require.GreaterOrEqual(t, len(found), 13, "every helper script call site is collected")
	for _, feature := range shellFeatures {
		if regexp.MustCompile(`(?m)^` + feature.flag + `=y$`).Match(config) {
			continue
		}
		for where, script := range found {
			assert.Falsef(t, feature.pattern.MatchString(script), "%s uses %s, which the tools shell lacks (%s unset)", where, feature.what, feature.flag)
		}
	}
}
