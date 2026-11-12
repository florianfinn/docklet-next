// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original guard against upstream release and image endpoints in product code.

package version

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// Closed criterion: no string literal in non-test Go source under backend/ or
// types/ names Arcane's release API or image registry path. Label namespaces
// such as com.getarcaneapp.* are out of scope.
var upstreamEndpointPrefixes = []string{"api.github.com/repos/" + "getarcaneapp", "ghcr.io/" + "getarcaneapp"}

func upstreamLiteralsInternal(t *testing.T, roots ...string) []string {
	t.Helper()
	var hits []string
	for _, root := range roots {
		err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if d.IsDir() {
				if name := d.Name(); name == "testdata" || name == "node_modules" || strings.HasPrefix(name, ".") && path != root {
					return filepath.SkipDir
				}
				return nil
			}
			if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
				return nil
			}
			file, err := parser.ParseFile(token.NewFileSet(), path, nil, parser.SkipObjectResolution)
			if err != nil {
				return err
			}
			ast.Inspect(file, func(n ast.Node) bool {
				lit, ok := n.(*ast.BasicLit)
				if !ok || lit.Kind != token.STRING {
					return true
				}
				for _, prefix := range upstreamEndpointPrefixes {
					if strings.Contains(lit.Value, prefix) {
						hits = append(hits, path+": "+lit.Value)
					}
				}
				return true
			})
			return nil
		})
		require.NoError(t, err)
	}
	return hits
}

func TestProductCodeNamesNoUpstreamReleaseOrImageEndpoint(t *testing.T) {
	require.Empty(t, upstreamLiteralsInternal(t, "../..", "../../../types"))
}

func TestUpstreamGuardDetectsALiteral(t *testing.T) {
	dir := t.TempDir()
	source := "package x\n\n// comments are ignored: " + upstreamEndpointPrefixes[1] + "\nconst ref = \"" + upstreamEndpointPrefixes[1] + "/arcane\"\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x.go"), []byte(source), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "x_test.go"), []byte("package x\n\nconst y = \""+upstreamEndpointPrefixes[0]+"\"\n"), 0o600))
	require.Len(t, upstreamLiteralsInternal(t, dir), 1)
}
