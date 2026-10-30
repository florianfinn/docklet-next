// Source: docklet-next/issues/294
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original test for volume usage cache isolation between fake daemons.

package volume

import (
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	arcanedocker "go.getarcane.app/docker"
)

// usageDaemonInternal serves one named volume from system/df; when addr is set it
// rebinds that address, reproducing a fake daemon that reuses an earlier test port.
func usageDaemonInternal(t *testing.T, addr, name string) *httptest.Server {
	t.Helper()
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/system/df") {
			_, _ = io.WriteString(w, `{"Volumes":[{"Name":"`+name+`","UsageData":{"Size":1,"RefCount":0}}]}`)
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	if addr != "" {
		listener, err := net.Listen("tcp", addr)
		require.NoError(t, err)
		_ = server.Listener.Close()
		server.Listener = listener
	}
	server.Start()
	t.Cleanup(server.Close)
	return server
}

func usageNamesInternal(t *testing.T, dockerClient *client.Client) []string {
	t.Helper()
	volumes, err := arcanedocker.GetVolumeUsageData(t.Context(), dockerClient)
	require.NoError(t, err)
	names := make([]string, 0, len(volumes))
	for _, v := range volumes {
		names = append(names, v.Name)
	}
	return names
}

func TestVolumeTestClientsIsolateUsageCacheAcrossReusedPorts(t *testing.T) {
	helpers := map[string]func(*testing.T, *httptest.Server) *client.Client{
		"service":   newVolumeServiceTestDockerClientInternal,
		"workspace": newVolumeWorkspaceTestDockerClientInternal,
	}
	for name, helper := range helpers {
		t.Run(name+"/creation", func(t *testing.T) {
			first := usageDaemonInternal(t, "", "first")
			raw, err := client.New(client.WithHost(first.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(first.Client()))
			require.NoError(t, err)
			t.Cleanup(func() { arcanedocker.InvalidateVolumeUsageCache(raw); _ = raw.Close() })
			require.Equal(t, []string{"first"}, usageNamesInternal(t, raw))
			addr := first.Listener.Addr().String()
			first.Close()

			second := usageDaemonInternal(t, addr, "second")
			require.Equal(t, []string{"second"}, usageNamesInternal(t, helper(t, second)))
		})
		t.Run(name+"/cleanup", func(t *testing.T) {
			var addr string
			t.Run("prime", func(t *testing.T) {
				first := usageDaemonInternal(t, "", "first")
				require.Equal(t, []string{"first"}, usageNamesInternal(t, helper(t, first)))
				addr = first.Listener.Addr().String()
			})
			second := usageDaemonInternal(t, addr, "second")
			raw, err := client.New(client.WithHost(second.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(second.Client()))
			require.NoError(t, err)
			t.Cleanup(func() { arcanedocker.InvalidateVolumeUsageCache(raw); _ = raw.Close() })
			require.Equal(t, []string{"second"}, usageNamesInternal(t, raw))
		})
	}
}
