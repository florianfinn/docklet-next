// Source: docklet-next/issues/346
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original shared schedule vectors and parser-boundary guard.

package schedule

import (
	"encoding/json/v2"
	"go/ast"
	goparser "go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestSharedScheduleRuleVectors(t *testing.T) {
	raw, err := os.ReadFile("../../../../types/settings/testdata/schedule-rule-vectors.json")
	require.NoError(t, err)
	var vectors []struct {
		Name       string   `json:"name"`
		Expression string   `json:"expression"`
		Zone       string   `json:"zone"`
		After      string   `json:"after"`
		Next       []string `json:"next"`
	}
	require.NoError(t, json.Unmarshal(raw, &vectors))
	for _, v := range vectors {
		t.Run(v.Name, func(t *testing.T) {
			loc, err := time.LoadLocation(v.Zone)
			require.NoError(t, err)
			schedule, err := Parse(v.Expression, loc)
			require.NoError(t, err)
			at, err := time.Parse(time.RFC3339, v.After)
			require.NoError(t, err)
			for _, next := range v.Next {
				got := schedule.Next(at)
				if next == "" {
					require.True(t, got.IsZero())
					break
				}
				want, err := time.Parse(time.RFC3339, next)
				require.NoError(t, err)
				require.True(t, want.Equal(got), "after %s: got %s, want %s", at, got, want)
				at = got
			}
		})
	}
}

func cronCallsOutsideEntry(source []byte) ([]string, error) {
	f, err := goparser.ParseFile(token.NewFileSet(), "input.go", source, 0)
	if err != nil {
		return nil, err
	}
	aliases := map[string]bool{}
	for _, imp := range f.Imports {
		path, err := strconv.Unquote(imp.Path.Value)
		if err != nil {
			return nil, err
		}
		if path == "github.com/robfig/cron/v3" {
			name := "cron"
			if imp.Name != nil {
				name = imp.Name.Name
			}
			aliases[name] = true
		}
	}
	var found []string
	if aliases["."] {
		found = append(found, "dot-import")
	}
	ast.Inspect(f, func(n ast.Node) bool {
		selector, ok := n.(*ast.SelectorExpr)
		if !ok {
			return true
		}
		base, ok := selector.X.(*ast.Ident)
		if !ok || !aliases[base.Name] {
			return true
		}
		switch selector.Sel.Name {
		case "New", "NewParser", "ParseStandard", "Parser":
			found = append(found, selector.Sel.Name)
		}
		return true
	})
	return found, nil
}

func TestOneScheduleParseEntryPoint(t *testing.T) {
	err := filepath.WalkDir("../../..", func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") || strings.Contains(filepath.ToSlash(path), "/scheduler/schedule/") {
			return nil
		}
		source, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		calls, err := cronCallsOutsideEntry(source)
		require.NoError(t, err, path)
		require.Empty(t, calls, "%s must use schedule.Parse", path)
		return nil
	})
	require.NoError(t, err)
}

func TestParserBoundaryRecognizesAliases(t *testing.T) {
	for _, call := range []string{"c.ParseStandard(expr)", "c.New()", "c.NewParser(0).Parse(expr)", "func(p c.Parser) { p.Parse(expr) }"} {
		calls, err := cronCallsOutsideEntry([]byte("package example; import c \"github.com/robfig/cron/v3\"; var _ = " + call))
		require.NoError(t, err)
		require.NotEmpty(t, calls, call)
	}
}
