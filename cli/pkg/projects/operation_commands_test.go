// Source: docklet-next/docs/design/runtime-control.md
// Revision: d43f9f48125be249a481dfffde7d8904107303b214c0d826423f4a119b9e76f4
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Command-level HTTP coverage for legacy pull and build stream selection.

package projects

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/cli/internal/config"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/runstate"
	"github.com/florianfinn/docklet-next/cli/internal/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestLegacyOperationCommandsReturnProducerFailure(t *testing.T) {
	for _, tc := range []struct {
		command *cobra.Command
		path    func(string, string) string
		prefix  string
	}{
		{pullCmd, types.ProjectPull, "failed to pull images"},
		{buildCmd, types.ProjectBuild, "failed to build project images"},
	} {
		t.Run(tc.command.Name(), func(t *testing.T) {
			message := tc.command.Name() + " access denied for example"
			requests := make(chan string, 4)
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests <- r.Method + " " + r.URL.Path
				switch {
				case r.Method == http.MethodGet && r.URL.Path == types.Project("0", "example-project"):
					w.Header().Set("Content-Type", "application/json")
					_, _ = fmt.Fprint(w, `{"success":true,"data":{"id":"example-project","name":"Example project"}}`)
				case r.Method == http.MethodPost && r.URL.Path == tc.path("0", "example-project"):
					w.Header().Set("Content-Type", "application/x-ndjson")
					// Match streamProjectOperationInternal: activity, Docker output,
					// then an uncoded error without a done frame on failure.
					_, _ = fmt.Fprintf(w, "{\"type\":\"activity\",\"activityId\":\"example-run\"}\n{\"log\":\"working\"}\n{\"error\":%q}\n", message)
				default:
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			previousPath, err := config.ConfigPath()
			require.NoError(t, err)
			require.NoError(t, config.SetConfigPath(filepath.Join(t.TempDir(), "config.yml")))
			t.Cleanup(func() { require.NoError(t, config.SetConfigPath(previousPath)) })
			require.NoError(t, config.Save(&types.Config{ServerURL: server.URL, APIKey: "example", DefaultEnvironment: "0"}))
			t.Setenv("ARCANE_TOKEN", "")
			previousState := runstate.Get()
			runstate.Set(runstate.State{})
			t.Cleanup(func() { runstate.Set(previousState) })

			var stdout, stderr bytes.Buffer
			output.Configure(&stdout, &stderr, true, true, "legacy")
			defer output.Configure(nil, nil, false, true, "")
			root := &cobra.Command{Use: "test", SilenceErrors: true, SilenceUsage: true}
			root.SetOut(&stdout)
			root.SetErr(&stderr)
			previousContext := tc.command.Context()
			ProjectsCmd.RemoveCommand(tc.command)
			root.AddCommand(tc.command)
			defer func() {
				root.RemoveCommand(tc.command)
				ProjectsCmd.AddCommand(tc.command)
				tc.command.SetContext(previousContext)
			}()
			root.SetArgs([]string{tc.command.Name(), "example-project"})
			ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
			defer cancel()
			err = root.ExecuteContext(ctx)
			require.EqualError(t, err, tc.prefix+": "+message, "command must return the legacy producer failure, not an interrupted-stream error")
			require.Equal(t, "working\n", stdout.String())
			require.Empty(t, stderr.String())
			require.Len(t, requests, 2)
			require.Equal(t, "GET "+types.Project("0", "example-project"), <-requests)
			require.Equal(t, "POST "+tc.path("0", "example-project"), <-requests)
		})
	}
}
