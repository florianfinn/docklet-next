// Source: docklet-next/issues/678
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original regression proofs.

package prune

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type deadlineTransport struct {
	t    *testing.T
	next http.RoundTripper
}

func (d deadlineTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	deadline, ok := r.Context().Deadline()
	require.True(d.t, ok)
	require.LessOrEqual(d.t, time.Until(deadline), time.Minute)
	return d.next.RoundTrip(r)
}
func TestPrunePlanUsesOneBoundedSnapshotWithoutRunningSizes(t *testing.T) {
	calls := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			calls++
			require.NotContains(t, r.URL.Query()["type"], "container")
			_, _ = w.Write([]byte(`{}`))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			if r.URL.Query().Get("size") == "1" {
				var filters client.Filters
				require.NoError(t, json.Unmarshal([]byte(r.URL.Query().Get("filters")), &filters))
				require.NotContains(t, filters["status"], "running")
				_, _ = w.Write([]byte(`[{"Id":"stopped","State":"exited","SizeRw":20}]`))
			} else {
				_, _ = w.Write([]byte(`[{"Id":"stopped","State":"exited"},{"Id":"running","State":"running"}]`))
			}
		case strings.HasSuffix(r.URL.Path, "/containers/stopped/json"):
			_, _ = w.Write([]byte(`{"Id":"stopped","State":{"Running":false}}`))
		default:
			t.Errorf("unexpected %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.52"), client.WithHTTPClient(&http.Client{Transport: deadlineTransport{t, http.DefaultTransport}}))
	require.NoError(t, err)
	defer cli.Close()
	svc := NewService(time.UTC, func(context.Context) (*client.Client, error) { return cli, nil }, nil, nil, nil, nil)
	plan, err := svc.preparePrunePlanInternal(t.Context(), system.PruneAllRequest{Containers: &system.PruneContainersOptions{Mode: system.PruneContainerModeStopped}})
	require.NoError(t, err)
	require.Len(t, plan.Resources, 1)
	require.EqualValues(t, 20, plan.Resources[0].Size)
	// Rechecks remain bounded but perform no second full disk query.
	ctx, cancel := context.WithTimeout(t.Context(), time.Minute)
	defer cancel()
	for range 3 {
		eligible, exists, err := svc.pruneResourceEligibleInternal(ctx, cli, plan, &plan.Resources[0])
		require.NoError(t, err)
		require.True(t, eligible)
		require.True(t, exists)
	}
	require.Equal(t, 1, calls)
}
