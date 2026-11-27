// Source: docklet-next/issues/674
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package details

import (
	"fmt"
	compose "github.com/compose-spec/compose-go/v2/types"
	image "github.com/florianfinn/docklet-next/types/image"
	project "github.com/florianfinn/docklet-next/types/project"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRuntimeImageIDRetainsConfiguredPolicyJoin(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		require.True(t, strings.HasSuffix(r.URL.Path, "/containers/app/json"))
		fmt.Fprint(w, `{"Id":"app","Config":{"Image":"example.com/app:1"}}`)
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"))
	require.NoError(t, err)
	defer cli.Close()
	listed := normalizeRuntimeImages(t.Context(), cli, []container.Summary{{ID: "app", Image: "sha256:" + strings.Repeat("a", 64)}})
	require.Equal(t, "example.com/app:1", listed[0].Image)
	info := ConfiguredRuntimeServiceUpdateInfo([]compose.ServiceConfig{{Name: "web", Image: "example.com/app:1"}}, []project.RuntimeService{{Name: "web", ContainerID: "app", Image: listed[0].Image}}, map[string]*image.UpdateInfo{"app": {HasUpdate: true}})
	require.NotNil(t, info["web"])
	require.True(t, info["web"].HasUpdate)
}
