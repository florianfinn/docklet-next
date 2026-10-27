// Source: docklet-next/docs/design/images-updates.md
// Revision: f6033a46c9ef4ad490661addd7dac41ab0ed5a45
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Scheduled prune head-image producer regression coverage.

package prune

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestScheduledPrunePlanExcludesIntermediateParents(t *testing.T) {
	parent := "sha256:" + strings.Repeat("a", 64)
	head := "sha256:" + strings.Repeat("b", 64)
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_, _ = fmt.Fprintf(w, `{"Images":[{"Id":%q},{"Id":%q}],"Containers":[],"Volumes":[]}`, parent, head)
		case strings.HasSuffix(r.URL.Path, "/images/json"):
			require.NotContains(t, []string{"true", "1"}, r.URL.Query().Get("all"))
			_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, head)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
		}
	}))
	defer daemon.Close()
	docker, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer docker.Close()
	service := NewService(time.UTC, func(context.Context) (*client.Client, error) { return docker, nil }, nil, nil, nil, nil)
	plan, err := service.preparePrunePlanInternal(t.Context(), system.PruneAllRequest{Images: &system.PruneImagesOptions{Mode: system.PruneImageModeAll}})
	require.NoError(t, err)
	require.Len(t, plan.Resources, 1)
	require.Equal(t, head, plan.Resources[0].ID)
}
