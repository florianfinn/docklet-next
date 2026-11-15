// Source: docklet-next/issues/440
// Revision: 92ca1a3ea43cfdc211ca561a968e09c8e8f747b1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed stack runtime, recovery, selection and cancellation proofs.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/stability"
	"github.com/florianfinn/docklet-next/backend/internal/stability/stabilitytest"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
	"gorm.io/gorm"
)

type stackTestClock struct {
	*stabilitytest.Clock
	tick  func()
	waits int
}

func (c *stackTestClock) Wait(ctx context.Context, until time.Time) error {
	c.waits++
	if c.tick != nil {
		c.tick()
	}
	return c.Clock.Wait(ctx, until)
}
func observedStackFixture(t *testing.T) (*lifecycleFixture, *stackTestClock) {
	f := newLifecycleFixture(t)
	clock := &stackTestClock{Clock: stabilitytest.NewClock()}
	f.service.UseObservationClock(clock)
	f.establishBaseline()
	return f, clock
}
func observedStackIntent(t *testing.T, f *lifecycleFixture, mode, kind string) *dto.LifecycleIntent {
	intent := f.intent(kind)
	if mode == "scheduled" {
		serviceScopePolicy(t, f, intent)
	}
	if mode == "manual" {
		intent.Request.AutoRestore = true
	}
	return intent
}
func requireStackRecovery(t *testing.T, out activityDTO.RuntimeSnapshot, outcome, code string) {
	t.Helper()
	require.Equal(t, activityDTO.StatusFailed, out.Status, "%+v", out.ImageLifecycle)
	require.NotNil(t, out.ImageLifecycle)
	require.False(t, out.ImageLifecycle.Verified)
	require.Equal(t, &dto.RecoveryResult{Outcome: outcome, Code: code}, out.ImageLifecycle.Recovery)
	require.NotEmpty(t, out.ImageLifecycle.RestorePointID)
}
func TestObservedStackDeployFailureRestoresOwnPoint(t *testing.T) {
	for _, mode := range []string{"scheduled", "manual"} {
		t.Run(mode, func(t *testing.T) {
			f, clock := observedStackFixture(t)
			intent := observedStackIntent(t, f, mode, "version")
			f.up = func(context.Context, *compose.Project) error {
				if f.deploys == 1 {
					return errors.New("synthetic deploy failure")
				}
				return nil
			}
			out := f.wait(f.admit(intent))
			requireStackRecovery(t, out, "restored", "")
			require.Equal(t, "update.deploy-failed", out.ImageLifecycle.Code)
			require.Equal(t, 2, f.deploys)
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, lifecycleOldID, f.workloads["app"].ImageID)
			require.Equal(t, stability.Window, clock.Elapsed())
			require.Len(t, lifecyclePoints(t, f), 1, "automatic recovery never prepares another point")
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			baseline, _, err := f.service.lifecycleStore().Baseline(t.Context(), current)
			require.NoError(t, err)
			require.Equal(t, lifecycleOldID, baseline.Services["app"].ImageID)
			// The next attempt sees the restored baseline instead of refusing its drift.
			retry, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
			// The fixture has no checker, so reaching the checker proves baseline readiness.
			require.Nil(t, retry)
			lifecycleCode(t, err, "update.check-unavailable")
			if mode == "manual" {
				require.Empty(t, out.ImageLifecycle.Applied)
			} else {
				require.Equal(t, []dto.AppliedCandidate{{Service: "app", Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: lifecycleDigest}}, out.ImageLifecycle.Applied)
				require.NotEqual(t, lifecycleNewID, out.ImageLifecycle.Applied[0].Manifest, "classic native config ID is not the pullable manifest")
			}
		})
	}
}
func TestObservedStackFreshSourceAndFullWindows(t *testing.T) {
	f, clock := observedStackFixture(t)
	intent := observedStackIntent(t, f, "scheduled", "version")
	f.up = func(context.Context, *compose.Project) error { clock.Jump(3 * time.Minute); return nil }
	clock.tick = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		c := f.workloads["app"]
		if f.deploys == 1 {
			c.Running = false
			f.workloads["app"] = c
		}
	}
	out := f.wait(f.admit(intent))
	requireStackRecovery(t, out, "restored", "")
	require.Equal(t, "update.unstable", out.ImageLifecycle.Code)
	require.Equal(t, 500, out.ImageLifecycle.HTTPStatus)
	require.Equal(t, 2, f.deploys)
	require.Equal(t, 6*time.Minute+stability.Interval+stability.Window, clock.Elapsed(), "slow deploy must not consume either observation window")
	require.Equal(t, lifecycleOldID, f.workloads["app"].ImageID)
}
func TestObservedStackSlowDeployGetsFullWindow(t *testing.T) {
	f, clock := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error { clock.Jump(3 * time.Minute); return nil }
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	require.True(t, out.ImageLifecycle.Verified)
	require.Equal(t, &dto.RecoveryResult{Outcome: "not-needed"}, out.ImageLifecycle.Recovery)
	require.Equal(t, 3*time.Minute+stability.Window, clock.Elapsed())
	require.Equal(t, 60, clock.waits)
}
func TestObservedStackStoppedSelectedServiceIsRequired(t *testing.T) {
	f := serviceScopeFixture(t, serviceScopeSource)
	c := f.workloads["app"]
	c.Running = false
	f.workloads["app"] = c
	f.establishBaseline()
	clock := &stackTestClock{Clock: stabilitytest.NewClock()}
	f.service.UseObservationClock(clock)
	intent := observedStackIntent(t, f, "scheduled", "version")
	intent.Request.Services = []string{"app"}
	clock.tick = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		c := f.workloads["app"]
		c.Running = false
		f.workloads["app"] = c
	}
	out := f.wait(f.admit(intent))
	requireStackRecovery(t, out, "restored", "")
	require.Equal(t, "update.unstable", out.ImageLifecycle.Code)
	require.Equal(t, 500, out.ImageLifecycle.HTTPStatus)
	require.Equal(t, 2, f.deploys)
	require.False(t, f.workloads["app"].Running)
}
func TestObservedStackRecoveryRequiresUnselectedRunningServices(t *testing.T) {
	f := serviceScopeFixture(t, serviceScopeSource)
	f.establishBaseline()
	clock := &stackTestClock{Clock: stabilitytest.NewClock()}
	f.service.UseObservationClock(clock)
	intent := observedStackIntent(t, f, "scheduled", "version")
	intent.Request.Services = []string{"app"}
	clock.tick = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		key := "app"
		if f.deploys == 2 {
			key = "running"
		}
		c := f.workloads[key]
		c.Running = false
		f.workloads[key] = c
	}
	out := f.wait(f.admit(intent))
	requireStackRecovery(t, out, "restore-failed", "restore.deploy-failed")
	require.Equal(t, "update.unstable", out.ImageLifecycle.Code)
	require.Equal(t, 500, out.ImageLifecycle.HTTPStatus)
	require.Equal(t, 2, f.deploys)
}
func TestObservedStackFailureBeforeDeployRestoresWithoutDeploy(t *testing.T) {
	for _, phase := range []string{"after-write", "retag-only"} {
		t.Run(phase, func(t *testing.T) {
			f, _ := observedStackFixture(t)
			kind := "version"
			if phase == "retag-only" {
				kind = "rebuild"
			}
			intent := observedStackIntent(t, f, "scheduled", kind)
			if phase == "after-write" {
				fired := false
				f.onInspect = func(ref string) {
					if !fired && ref == "registry.example.com/app:1.1.0" && strings.Contains(lifecycleFile(t, f, "compose.yaml"), "app:1.1.0") {
						fired = true
						f.mu.Lock()
						f.tags[ref] = lifecycleOldID
						f.mu.Unlock()
					}
				}
			} else {
				armed := false
				f.onInspect = func(ref string) {
					f.mu.Lock()
					defer f.mu.Unlock()
					if ref == "registry.example.com/app:1.0.0" && f.tags[ref] == lifecycleNewID {
						armed = true
					}
				}
				require.NoError(t, f.db.Callback().Update().Before("gorm:update").Register("s4_invalidate", func(tx *gorm.DB) {
					if armed && tx.Statement.Table == "deployment_baselines" {
						armed = false
						f.onInspect = nil
						tx.AddError(errors.New("synthetic invalidate failure"))
					}
				}))
				t.Cleanup(func() { _ = f.db.Callback().Update().Remove("s4_invalidate") })
			}
			out := f.wait(f.admit(intent))
			requireStackRecovery(t, out, "restored", "")
			require.Zero(t, f.deploys)
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, lifecycleOldID, f.tags["registry.example.com/app:1.0.0"])
			require.Len(t, lifecyclePoints(t, f), 1, "the committed selection effect retains its point after settlement")
		})
	}
}
func TestObservedStackProvenanceRefusesUnknownBytes(t *testing.T) {
	for _, path := range []string{"service.env", ".env", "compose.yaml"} {
		t.Run(path, func(t *testing.T) {
			f, _ := observedStackFixture(t)
			intent := observedStackIntent(t, f, "scheduled", "rebuild")
			tagsAtFailure := 0
			f.up = func(context.Context, *compose.Project) error {
				tagsAtFailure = f.tagWrites
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, path), []byte("# external edit\n"), 0600))
				return errors.New("synthetic deploy failure")
			}
			out := f.wait(f.admit(intent))
			requireStackRecovery(t, out, "unavailable", "restore.files-changed")
			require.Equal(t, "# external edit\n", lifecycleFile(t, f, path))
			require.Equal(t, tagsAtFailure, f.tagWrites, "a refused helper must not run exit-defer reconciliation")
			require.Equal(t, lifecycleNewID, f.tags["registry.example.com/app:1.0.0"])
			require.Equal(t, 1, f.deploys)
		})
	}
}
func TestObservedStackTornWriterRefusesRecovery(t *testing.T) {
	for _, shape := range []string{"valid-truncated", "invalid-yaml"} {
		t.Run(shape, func(t *testing.T) {
			f, _ := observedStackFixture(t)
			writer := lifecycleWriteFile
			t.Cleanup(func() { lifecycleWriteFile = writer })
			torn := ""
			lifecycleWriteFile = func(ctx context.Context, root, path string, body []byte, options acfs.WriteOptions) error {
				require.NoError(t, writer(ctx, root, path, body, options))
				if path != "/compose.yaml" {
					return nil
				}
				torn = string(body[:len(body)-1])
				if shape == "invalid-yaml" {
					torn = "services: ["
				}
				require.NoError(t, os.WriteFile(filepath.Join(root, strings.TrimPrefix(path, "/")), []byte(torn), 0600))
				return errors.New("synthetic torn in-place write")
			}
			out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
			requireStackRecovery(t, out, "unavailable", "restore.files-changed")
			require.Equal(t, torn, lifecycleFile(t, f, "compose.yaml"))
			require.Zero(t, f.deploys)
		})
	}
}
