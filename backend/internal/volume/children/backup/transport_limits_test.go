// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original registered-route download size and synchronous deletion cancellation proofs.

package backup

import (
	"context"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestDownloadLimitsThroughHubDirectAndActualEdgeRoutes(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, bound := range []string{"unknown", "pre-size", "post-size", "empty", "success"} {
			t.Run(mode+"/"+bound, func(t *testing.T) {
				f := newRuntimePlan(t)
				size := int64(1)
				if bound == "empty" {
					size = 0
				}
				if bound == "unknown" {
					size = -1
				}
				if bound == "pre-size" {
					size = dto.MaxEdgeDownloadBytes + 1
				}
				if bound == "post-size" && mode == "edge" {
					f.packaged = dto.MaxEdgeDownloadBytes + 1
				}
				row := &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, Destination: "local", LocalSnapshotID: strings.Repeat("a", 64), Size: size}
				f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) { return row, nil }
				router, rawAPI := humatest.New(t)
				api := huma.NewGroup(rawAPI, "/api")

				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
					request = context.WithValue(request, user.CurrentUserContextKey{}, &user.SystemUser)
					next(huma.WithContext(ctx, request))
				})
				cfg := &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}
				RegisterRoutes(api, NewHandler(f.service, nil, nil, nil, t.Context(), cfg))
				call := backupRouteTransport(t, mode, router)
				status, _, body := call(t.Context(), "GET", "/api/environments/0/volumes/backups/row/download", nil)
				if mode == "edge" && bound != "success" && bound != "empty" {
					require.Equal(t, 413, status, string(body))
					require.Contains(t, string(body), "volume.backup.download-too-large")
					if bound == "unknown" || bound == "pre-size" {
						require.Empty(t, f.creates)
					} else {
						require.Contains(t, f.removed, "scratch")
					}
				} else {
					require.Equal(t, 200, status, string(body))
					require.Equal(t, "packed", string(body))
					require.Contains(t, f.removed, "scratch")
				}
				unlock, ok := f.service.deps.Locks.TryLock("data")
				require.True(t, ok, "registered response completion must release workspace ownership")
				unlock()
			})
		}
	}
}

func TestSynchronousDeleteHandlerCancelsEngineOnRequestEnd(t *testing.T) {
	f := newRuntimePlan(t)
	entered, cancelled := make(chan struct{}), make(chan struct{})
	fallbackRelease := make(chan struct{})
	defer close(fallbackRelease)
	original := f.serve
	// Replace only Docker start with a held daemon request; cleanup still uses
	// the actual runtime's remove + inspect path against this synthetic daemon.
	restarted := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/start") {
			close(entered)
			select {
			case <-r.Context().Done():
				close(cancelled)
			case <-fallbackRelease:
				w.WriteHeader(503)
			}
			return
		}
		original(w, r)
	}), &restarted)
	s.deps.Engine = f.service.deps.Engine
	s.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
		return &volume.Backup{ID: "row", VolumeName: "data", Format: volume.BackupFormatRustic, Destination: "local", LocalSnapshotID: strings.Repeat("a", 64)}, nil
	}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	h := NewHandler(s, nil, nil, nil, t.Context(), nil)
	done := make(chan error, 1)
	go func() {
		_, err := h.DeleteBackup(ctx, &DeleteBackupInput{BackupID: "row", EnvironmentID: "0"})
		done <- err
	}()
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("delete never reached held engine")
	}
	cancel()
	require.Eventually(t, func() bool {
		select {
		case <-cancelled:
			return true
		default:
			return false
		}
	}, 500*time.Millisecond, 5*time.Millisecond, "delete detached from request cancellation")
	select {
	case err := <-done:
		require.Error(t, err)
	case <-time.After(2 * time.Second):
		t.Fatal("delete did not join cleanup")
	}
	require.Contains(t, f.removed, "helper")
	unlock, ok := s.deps.Locks.TryLock("data")
	require.True(t, ok)
	unlock()
}

func TestEngineOutputProblemsStayTypedAndDiagnosticFreeThroughAllTransports(t *testing.T) {
	for _, mode := range []string{"hub", "direct", "edge"} {
		for _, fault := range []string{"malformed", "overflow", "diagnostic"} {
			t.Run(mode+"/"+fault, func(t *testing.T) {
				f := newRuntimePlan(t)
				f.service.deps.Store.Run = func(context.Context, string) (*volume.Backup, error) {
					return &volume.Backup{ID: "row", VolumeName: "data", Format: "rustic", Destination: "local", LocalSnapshotID: strings.Repeat("a", 64)}, nil
				}
				f.service = NewService(f.service.deps)
				expected := "volume.backup.engine-output-invalid"
				switch fault {
				case "malformed":
					f.stdout = `{"unexpected":"shape"}`
				case "overflow":
					f.stdout = strings.Repeat(" ", dto.MaxVolumeResponseBytes+1)
					expected = "volume.backup.listing-too-large"
				case "diagnostic":
					f.stdout = ""
					f.stderr = "synthetic-password-sentinel at /srv/diagnostic/example"
					f.exit = 1
					expected = "volume.backup.repository-unavailable"
				}
				router, rawAPI := humatest.New(t)
				api := huma.NewGroup(rawAPI, "/api")
				api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
					next(huma.WithContext(ctx, context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())))
				})
				RegisterRoutes(api, NewHandler(f.service, nil, nil, nil, t.Context(), &config.Config{AgentMode: mode == "direct", EdgeAgent: mode == "edge"}))
				status, _, body := backupRouteTransport(t, mode, router)(t.Context(), "GET", "/api/environments/0/volumes/backups/row/files", nil)
				require.GreaterOrEqual(t, status, 400, string(body))
				require.Contains(t, string(body), expected)
				require.NotContains(t, string(body), "synthetic-password-sentinel")
				require.NotContains(t, string(body), "/srv/diagnostic")
				require.Contains(t, f.removed, "helper", "output refusal must still confirm engine cleanup")
			})
		}
	}
}
