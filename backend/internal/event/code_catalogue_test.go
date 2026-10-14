// Source: florianfinn/docklet-next/backend/internal/event/model.go
// Revision: d4e7845e461f51d56a63d3a43937679d6449cf3c
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: Guard event-type coverage of the shared message-code catalogue.

package event

import (
	"go/ast"
	"go/parser"
	"go/token"
	"strconv"
	"testing"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestEveryEventTypeHasMessageCode(t *testing.T) {
	parsed, err := parser.ParseFile(token.NewFileSet(), "model.go", nil, 0)
	require.NoError(t, err)
	count := 0
	ast.Inspect(parsed, func(node ast.Node) bool {
		entry, ok := node.(*ast.ValueSpec)
		if !ok || entry.Type == nil {
			return true
		}
		kind, ok := entry.Type.(*ast.Ident)
		if !ok || kind.Name != "EventType" {
			return true
		}
		for _, value := range entry.Values {
			literal, ok := value.(*ast.BasicLit)
			require.True(t, ok)
			code, unquoteErr := strconv.Unquote(literal.Value)
			require.NoError(t, unquoteErr)
			require.True(t, activity.Valid(code), code)
			count++
		}
		return true
	})
	require.Equal(t, 75, count)
}
