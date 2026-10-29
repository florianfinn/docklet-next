// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original truthful asynchronous restore CLI HTTP fixtures.

package volumes

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/cli/internal/client"
	"github.com/florianfinn/docklet-next/cli/internal/config"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/runstate"
	"github.com/florianfinn/docklet-next/cli/internal/types"
	"github.com/spf13/cobra"
	"github.com/stretchr/testify/require"
)

func TestRestoreCLIReportsAcceptedIDsAndPreservesRefusal(t *testing.T) {
	for _, kind := range []string{"full", "files", "upload"} {
		for _, jsonMode := range []bool{false, true} {
			for _, refuse := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/json=%t/refuse=%t", kind, jsonMode, refuse), func(t *testing.T) {
					posts := 0
					server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						w.Header().Set("Content-Type", "application/json")
						switch {
						case r.Method == http.MethodGet && r.URL.Path == types.Volume("0", "app-data"):
							_, _ = w.Write([]byte(`{"success":true,"data":{"name":"app-data"}}`))
						case r.Method == http.MethodPost && r.URL.Path == "/api/environments/0/uploads/volume-backup":
							_, _ = w.Write([]byte(`{"success":true,"data":{"id":"session","chunkSize":5242880,"totalChunks":1}}`))
						case r.Method == http.MethodPut:
							_, _ = w.Write([]byte(`{"success":true,"data":{"id":"session","complete":true}}`))
						case r.Method == http.MethodDelete:
							w.WriteHeader(204)
						case r.Method == http.MethodPost:
							posts++
							if refuse {
								w.WriteHeader(409)
								_, _ = w.Write([]byte(`{"status":409,"title":"Busy","detail":"volume.backup.busy","code":"volume.backup.busy"}`))
								return
							}
							w.WriteHeader(202)
							_, _ = w.Write([]byte(`{"success":true,"data":{"activityId":"activity-example","restoreId":"restore-example"}}`))
						case r.Method == http.MethodGet && r.URL.Path == "/api/environments/0/activities/activity-example":
							_, _ = w.Write([]byte(`{"success":true,"data":{"id":"activity-example","status":"failed","error":"volume.restore.failed-partial"}}`))
						default:
							t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
							http.NotFound(w, r)
						}
					}))
					defer server.Close()
					var stdout, stderr bytes.Buffer
					output.Configure(&stdout, &stderr, true, true, "legacy")
					defer output.Configure(nil, nil, false, true, "")
					oldForce, oldJSON, oldPaths, oldArchive := forceFlag, jsonOutput, restorePaths, restoreArchiveFile
					forceFlag = true
					jsonOutput = jsonMode
					restorePaths = []string{"file"}
					restoreArchiveFile = filepath.Join(t.TempDir(), "input.tgz")
					require.NoError(t, os.WriteFile(restoreArchiveFile, []byte("fixture"), 0600))
					defer func() {
						forceFlag = oldForce
						jsonOutput = oldJSON
						restorePaths = oldPaths
						restoreArchiveFile = oldArchive
					}()
					c, err := client.New(&types.Config{ServerURL: server.URL, APIKey: "example", DefaultEnvironment: "0"})
					require.NoError(t, err)
					c.SetRetryPolicy(1, 0, 0)
					cmd := &cobra.Command{}
					cmd.SetContext(t.Context())
					switch kind {
					case "files":
						err = runRestorePaths(cmd, c, "app-data", "backup-example")
					case "upload":
						err = runUploadRestore(cmd, c, "app-data")
					case "full":
						previousPath, pathErr := config.ConfigPath()
						require.NoError(t, pathErr)
						require.NoError(t, config.SetConfigPath(filepath.Join(t.TempDir(), "config.yml")))
						defer config.SetConfigPath(previousPath)
						require.NoError(t, config.Save(&types.Config{ServerURL: server.URL, APIKey: "example", DefaultEnvironment: "0"}))
						t.Setenv("ARCANE_TOKEN", "")
						state := runstate.Get()
						runstate.Set(runstate.State{})
						defer runstate.Set(state)
						restorePaths = nil
						restoreArchiveFile = ""
						err = backupsRestoreCmd.RunE(cmd, []string{"app-data", "backup-example"})
					}
					require.Equal(t, 1, posts)
					if refuse {
						require.ErrorContains(t, err, "volume.backup.busy")
						require.NotContains(t, stdout.String(), "Restore accepted")
						return
					}
					require.NoError(t, err)
					require.Contains(t, stdout.String(), "activity-example")
					require.Contains(t, stdout.String(), "restore-example")
					require.NotContains(t, stdout.String(), "restored successfully")
					if jsonMode {
						require.Contains(t, stdout.String(), `"activityId"`)
						require.Contains(t, stdout.String(), `"restoreId"`)
					} else {
						require.Contains(t, stdout.String(), "Restore accepted (activity activity-example, restore restore-example)")
					}
					// Acceptance remains truthful even when the followable activity later fails;
					// the command does not infer successful restoration or repeat the POST.
					request, err := http.NewRequestWithContext(context.Background(), http.MethodGet, server.URL+"/api/environments/0/activities/activity-example", nil)
					require.NoError(t, err)
					response, err := http.DefaultClient.Do(request)
					require.NoError(t, err)
					require.NoError(t, response.Body.Close())
					require.Equal(t, 1, posts)
				})
			}
		}
	}
}
