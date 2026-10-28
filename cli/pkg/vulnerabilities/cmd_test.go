// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: c477a3d9aba28f8ae7ab6683b56e380a36fa1aa1c6d8e7de3bffeb4b61b7305f
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Verify the CLI scan policy at the actual HTTP request boundary.

package vulnerabilities

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/cli/internal/config"
	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/florianfinn/docklet-next/cli/internal/runstate"
	"github.com/florianfinn/docklet-next/cli/internal/types"
	dto "github.com/florianfinn/docklet-next/types/vulnerability"
	"github.com/stretchr/testify/require"
)

func TestScanIgnoreUnfixedDefaultsFalseAndFlagReachesAgent(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	payloads := make(chan dto.ImageScanRequest, 2)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" && r.URL.Path == types.Image("0", id) {
			_, _ = fmt.Fprintf(w, `{"success":true,"data":{"id":%q}}`, id)
			return
		}
		if r.Method == "POST" && r.URL.Path == types.ImageVulnerabilitiesScan("0", id) {
			var payload dto.ImageScanRequest
			require.NoError(t, json.NewDecoder(r.Body).Decode(&payload))
			payloads <- payload
			w.WriteHeader(202)
			_, _ = fmt.Fprint(w, `{"success":true,"data":{"activityId":"scan"}}`)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		http.NotFound(w, r)
	}))
	defer server.Close()
	previousPath, e := config.ConfigPath()
	require.NoError(t, e)
	require.NoError(t, config.SetConfigPath(filepath.Join(t.TempDir(), "config.yml")))
	t.Cleanup(func() { require.NoError(t, config.SetConfigPath(previousPath)) })
	require.NoError(t, config.Save(&types.Config{ServerURL: server.URL, APIKey: "example", DefaultEnvironment: "0"}))
	t.Setenv("ARCANE_TOKEN", "")
	previousState := runstate.Get()
	runstate.Set(runstate.State{})
	defer runstate.Set(previousState)
	previousContext := scanCmd.Context()
	scanCmd.SetContext(t.Context())
	defer scanCmd.SetContext(previousContext)
	var stdout, stderr bytes.Buffer
	output.Configure(&stdout, &stderr, true, true, "")
	defer output.Configure(nil, nil, false, true, "")
	defer func() { _ = scanCmd.Flags().Set("ignore-unfixed", "false"); _ = scanCmd.Flags().Set("json", "false") }()
	require.Equal(t, "false", scanCmd.Flags().Lookup("ignore-unfixed").DefValue)
	for _, value := range []bool{false, true} {
		require.NoError(t, scanCmd.ParseFlags([]string{"--ignore-unfixed=" + fmt.Sprint(value), "--json"}))
		require.NoError(t, scanCmd.RunE(scanCmd, []string{id}))
		request := <-payloads
		require.Equal(t, value, request.IgnoreUnfixed)
		require.Equal(t, "manual", request.Origin)
	}
}
