// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Real HTTP proof for the five synchronous lifecycle message callers.

package cmdutil

import (
	"bytes"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/florianfinn/docklet-next/cli/internal/client"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/types"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRuntimeMessageCallersRejectEveryFailureStatus(t *testing.T) {
	paths := []string{types.ProjectDown("0", "example-stack"), types.ProjectRestart("0", "example-stack"), types.ContainerStart("0", "example-container"), types.ContainerStop("0", "example-container"), types.ContainerRestart("0", "example-container")}
	for _, path := range paths {
		for _, status := range []int{200, 408, 409, 500, 503, 504} {
			t.Run(fmt.Sprintf("%s/%d", path, status), func(t *testing.T) {
				calls := 0
				server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					require.Equal(t, http.MethodPost, r.Method)
					require.Equal(t, path, r.URL.Path)
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(status)
					if status == 200 {
						_, _ = fmt.Fprint(w, `{"success":true,"data":{"message":"completed","activityId":"example-run"}}`)
					} else {
						_, _ = fmt.Fprint(w, `{"success":false,"error":{"code":"runtime.failed","message":"runtime.failed"},"data":{"activityId":"example-run"}}`)
					}
				}))
				defer server.Close()
				c, err := client.NewUnauthenticated(&types.Config{ServerURL: server.URL})
				require.NoError(t, err)
				var stdout, stderr bytes.Buffer
				output.Configure(&stdout, &stderr, true, true, "runtime")
				defer output.Configure(nil, nil, false, true, "")
				cmd := &cobra.Command{Short: "runtime"}
				cmd.SetContext(t.Context())
				cmd.SetOut(&stdout)
				cmd.SetErr(&stderr)
				err = RunPostAction[base.MessageResponse](cmd, c, PostActionSpec{Path: path, FailureMessage: "action failed", SuccessMessage: "confirmed runtime completion"})
				if status == 200 {
					require.NoError(t, err)
					require.Contains(t, stdout.String(), "confirmed runtime completion")
				} else {
					require.Error(t, err)
					require.NotContains(t, stdout.String(), "confirmed runtime completion")
				}
				require.Equal(t, 1, calls, "lifecycle POST must not retry Execute")
			})
		}
	}
}
