// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original typed Compose load problem tests for issue 547.

package projects

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/compose-spec/compose-go/v2/template"
	"github.com/stretchr/testify/require"
)

const sentinel = "example-sentinel-value"

func TestClassifyLoadErrorNamesTheVariableWithoutItsMessage(t *testing.T) {
	dir := t.TempDir()
	compose := filepath.Join(dir, "compose.yaml")
	require.NoError(t, os.WriteFile(compose, []byte("name: edge\nservices:\n  app:\n    image: registry.example.com/app:${APP_TAG:?"+sentinel+"}\n"), 0600))
	_, err := LoadComposeProject(t.Context(), compose, "edge", dir, false, nil, nil, nil, false, nil, nil, nil)
	require.Error(t, err)
	var missing *template.MissingRequiredError
	require.ErrorAs(t, err, &missing, "the cause stays reachable")
	require.Contains(t, missing.Reason, sentinel, "precondition: the cause carries the operator message")
	require.NotContains(t, fmt.Errorf("wrapped: %w", err).Error(), sentinel, "printing never reveals it")
	problem := ClassifyLoadError(fmt.Errorf("wrapped: %w", err))
	require.Equal(t, LoadProblem{Reason: "compose.variable-missing", Variable: "APP_TAG"}, problem)
	require.False(t, strings.Contains(problem.String(), sentinel))
}

func TestClassifyLoadErrorClasses(t *testing.T) {
	require.Equal(t, "compose.env-file-missing", ClassifyLoadError(fmt.Errorf("x: %w", os.ErrNotExist)).Reason)
	require.Equal(t, "compose.selection-unsupported", ClassifyLoadError(fmt.Errorf("x: %w", ErrComposeFileNestedBase)).Reason)
	require.Equal(t, "compose.identity-unreadable", ClassifyLoadError(fmt.Errorf("line 3: "+sentinel)).Reason)
	require.Equal(t, LoadProblem{}, ClassifyLoadError(nil))
}
