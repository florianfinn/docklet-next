// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package guard

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const imageA = "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
const imageB = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
const imageC = "sha256:cccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccccc"

func testGuard(t *testing.T, handler http.HandlerFunc) *Service {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "images.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	c, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = c.Close() })
	return &Service{DB: db, Admission: gate, Docker: func(context.Context) (*client.Client, error) { return c, nil }}
}
func apiPath(r *http.Request) string {
	p := r.URL.Path
	if strings.HasPrefix(p, "/v") {
		p = p[strings.Index(p[1:], "/")+1:]
	}
	return p
}
func TestGuardProtectsAllAliasesAndStoppedConsumers(t *testing.T) {
	for _, scenario := range []string{"running", "stopped", "system", "restore-point", "active-update", "system-reference", "unknown-usage"} {
		t.Run(scenario, func(t *testing.T) {
			var deletes atomic.Int32
			g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				p := apiPath(r)
				if r.Method == http.MethodDelete {
					deletes.Add(1)
					_, _ = fmt.Fprint(w, `[]`)
					return
				}
				if strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json") {
					_, _ = fmt.Fprintf(w, `{"Id":%q,"Size":20}`, imageA)
					return
				}
				if p == "/containers/json" {
					require.Contains(t, []string{"true", "1"}, r.URL.Query().Get("all"))
					if scenario == "unknown-usage" {
						http.Error(w, "synthetic private diagnostic", 500)
						return
					}
					if scenario == "running" || scenario == "stopped" || scenario == "system" {
						state := "running"
						if scenario == "stopped" {
							state = "exited"
						}
						_, _ = fmt.Fprintf(w, `[{"Id":"container","ImageID":%q,"State":%q,"Labels":{"com.getarcaneapp.internal.resource":%q}}]`, imageA, state, fmt.Sprint(scenario == "system"))
						return
					}
					_, _ = fmt.Fprint(w, `[]`)
					return
				}
				t.Errorf("unexpected request %s", p)
			})
			if scenario == "restore-point" || scenario == "active-update" {
				require.NoError(t, g.Protect(t.Context(), Protection{ImageID: imageA, Reason: scenario, OwnerID: "owner"}))
			}
			if scenario == "system-reference" {
				g.SystemReferences = func() []string { return []string{"registry.example.com/tools:1.0.0"} }
			}
			release, err := g.Acquire(t.Context())
			require.NoError(t, err)
			defer release()
			c, err := g.Docker(t.Context())
			require.NoError(t, err)
			_, err = g.RemoveLocked(t.Context(), c, "registry.example.com/alias:1.0.0")
			assert.Error(t, err)
			require.Zero(t, deletes.Load(), "protected alias must not reach ImageRemove")
		})
	}
}
func TestGuardPrunePreservesPinsAndPartialReports(t *testing.T) {
	var deletes atomic.Int32
	var daemonPrunes atomic.Int32
	g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := apiPath(r)
		if p == "/images/prune" {
			daemonPrunes.Add(1)
			_, _ = fmt.Fprint(w, `{}`)
			return
		}
		if p == "/images/json" {
			_, _ = fmt.Fprintf(w, `[{"Id":%q},{"Id":%q},{"Id":%q},{"Id":%q}]`, imageA, imageB, imageC, imageB)
			return
		}
		if p == "/containers/json" {
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		if strings.HasPrefix(p, "/images/") && strings.HasSuffix(p, "/json") {
			id := strings.TrimSuffix(strings.TrimPrefix(p, "/images/"), "/json")
			_, _ = fmt.Fprintf(w, `{"Id":%q,"Size":100}`, id)
			return
		}
		if r.Method == http.MethodDelete {
			deletes.Add(1)
			require.NotEqual(t, "true", r.URL.Query().Get("force"))
			require.NotEqual(t, "1", r.URL.Query().Get("force"))
			require.Contains(t, []string{"true", "1"}, r.URL.Query().Get("noprune"))
			if strings.HasSuffix(p, imageC) {
				http.Error(w, "synthetic raw failure", 500)
				return
			}
			_, _ = fmt.Fprintf(w, `[{"Deleted":%q}]`, imageB)
			return
		}
		t.Errorf("unexpected %s", p)
	})
	require.NoError(t, g.Protect(t.Context(), Protection{ImageID: imageA, Reason: "restore-point", OwnerID: "point"}))
	report, err := g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.NoError(t, err)
	require.Equal(t, []string{imageB}, report.ImagesDeleted)
	require.Equal(t, []imageDTO.PruneItem{{ID: imageA, Code: "image.restore-point-protected"}}, report.Skipped)
	require.Equal(t, []imageDTO.PruneItem{{ID: imageC, Code: "image.remove-failed"}}, report.Failed)
	require.False(t, report.SpaceReclaimedKnown)
	require.Zero(t, report.SpaceReclaimed)
	require.EqualValues(t, 2, deletes.Load())
	require.Zero(t, daemonPrunes.Load())
	data, err := json.Marshal(report)
	require.NoError(t, err)
	require.NotContains(t, string(data), "synthetic raw failure")
}
func TestGuardBusyHasZeroDaemonCalls(t *testing.T) {
	var calls atomic.Int32
	g := testGuard(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); http.Error(w, "unexpected", 500) })
	release, err := g.Acquire(t.Context())
	require.NoError(t, err)
	defer release()
	_, err = g.Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	assert.Equal(t, "image.busy", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Zero(t, calls.Load())
}
func TestGuardIdentityRecheckPreventsRemoval(t *testing.T) {
	var inspect, deletes atomic.Int32
	g := testGuard(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deletes.Add(1)
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		if apiPath(r) == "/containers/json" {
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		id := imageA
		if inspect.Add(1) > 1 {
			id = imageB
		}
		_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
	})
	c, err := g.Docker(t.Context())
	require.NoError(t, err)
	_, err = g.RemoveLocked(t.Context(), c, "registry.example.com/app:1.0.0")
	assert.Equal(t, "image.changed", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Zero(t, deletes.Load())
}
