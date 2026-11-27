// Source: docklet-next/issues/678
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package docker

import (
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDefaultNetworksCountAsInUse(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/containers/json") {
			_, _ = w.Write([]byte(`[]`))
		} else {
			_, _ = w.Write([]byte(`[{"Id":"builtin","Name":"bridge"},{"Id":"unused","Name":"example"}]`))
		}
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
	require.NoError(t, err)
	defer cli.Close()
	svc := NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli)
	_, used, unused, total, err := svc.GetAllNetworks(t.Context())
	require.NoError(t, err)
	require.Equal(t, 1, used)
	require.Equal(t, 1, unused)
	require.Equal(t, total, used+unused)
}
