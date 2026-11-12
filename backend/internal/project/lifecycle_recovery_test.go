// Source: docklet-next/issues/440
// Revision: 92ca1a3ea43cfdc211ca561a968e09c8e8f747b1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original real lifecycle recovery guard, uncertain effect and cancellation fault vectors.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
)

func TestObservedStackRecoveryGuardsBeforeAnyRetag(t *testing.T) {
	for _, fault := range []string{"pin", "lease", "shared-reference", "point-anchor"} {
		t.Run(fault, func(t *testing.T) {
			f, _ := observedStackFixture(t)
			intent := observedStackIntent(t, f, "scheduled", "rebuild")
			beforeTags := 0
			f.up = func(context.Context, *compose.Project) error {
				if f.deploys > 1 {
					return nil
				}
				beforeTags = f.tagWrites
				switch fault {
				case "pin":
					f.mu.Lock()
					for tag := range f.tags {
						if strings.Contains(tag, "/points/") {
							delete(f.tags, tag)
						}
					}
					f.mu.Unlock()
				case "lease":
					release, err := f.service.imageService.ImageGuard().Acquire(t.Context())
					require.NoError(t, err)
					t.Cleanup(release)
				case "shared-reference":
					f.mu.Lock()
					f.workloads["peer"] = lifecycleWorkload{ID: strings.Repeat("e", 64), Service: "app", Project: "other-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleNewID}
					f.mu.Unlock()
				case "point-anchor":
					require.NoError(t, f.db.Model(&life.Point{}).Where("target_id = ?", f.project.ID).Update("source_run_id", "another-run").Error)
				}
				return errors.New("synthetic deploy failure")
			}
			out := f.wait(f.admit(intent))
			code := "restore.unavailable"
			if fault == "lease" {
				code = "image.busy"
			}
			if fault == "shared-reference" {
				code = "restore.shared-reference"
			}
			requireStackRecovery(t, out, "unavailable", code)
			require.Equal(t, "update.deploy-failed", out.ImageLifecycle.Code)
			require.Equal(t, beforeTags, f.tagWrites)
			require.Equal(t, lifecycleNewID, f.tags["registry.example.com/app:1.0.0"])
			require.Equal(t, 1, f.deploys)
		})
	}
}
func TestObservedStackPartialRecoverySkipsRebuildDefer(t *testing.T) {
	f, _ := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
	recoveryTags := 0
	f.onTag = func(source string) error {
		if f.deploys == 1 && source == lifecycleOldID {
			recoveryTags++
			if recoveryTags == 2 {
				return errors.New("synthetic restore target retag failure")
			}
		}
		return nil
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "rebuild")))
	requireStackRecovery(t, out, "restore-failed", "image.tag-failed")
	require.Equal(t, 2, recoveryTags, "helper pin and failed target retag only; no exit-defer retag")
	require.Equal(t, lifecycleNewID, f.tags["registry.example.com/app:1.0.0"])
	require.Equal(t, 1, f.deploys)
}
func TestObservedStackUncertainRecoveryHoldsRuntime(t *testing.T) {
	f, _ := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			return errors.New("synthetic deploy failure")
		}
		return &dto.UncertainImageEffect{Cause: errors.New("synthetic uncertain recovery effect")}
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	require.Equal(t, activityDTO.RuntimeOutcomeUnknown, out.OutcomeState)
	require.Nil(t, out.ImageLifecycle.Recovery)
	require.NotEmpty(t, out.ImageLifecycle.RestorePointID)
	held, err := f.service.lifecycleStore().Held(t.Context(), out.ImageLifecycle.RestorePointID)
	require.NoError(t, err)
	require.True(t, held)
	require.Equal(t, 2, f.deploys)
}
func TestObservedStackRecoveryBaselineFailureDoesNotUndoProof(t *testing.T) {
	f, _ := observedStackFixture(t)
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			return errors.New("synthetic deploy failure")
		}
		require.NoError(t, f.db.Exec(`CREATE TRIGGER s4_fail_baseline BEFORE INSERT ON deployment_baselines BEGIN SELECT RAISE(ABORT,'synthetic save failure'); END`).Error)
		return nil
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	requireStackRecovery(t, out, "restored", "")
	require.Equal(t, "update.baseline-unavailable", out.ImageLifecycle.BaselineCode)
	require.Equal(t, lifecycleOldID, f.workloads["app"].ImageID)
}
func TestLifecyclePointVisibilityTracksCommittedSelectionEffect(t *testing.T) {
	for _, mode := range []string{"plain", "scheduled"} {
		for _, phase := range []string{"pull", "before-effect", "selection-effect"} {
			t.Run(mode+"/"+phase, func(t *testing.T) {
				f, _ := observedStackFixture(t)
				intent := observedStackIntent(t, f, mode, "rebuild")
				if phase == "pull" {
					f.onPull = func() {
						f.mu.Lock()
						for tag, id := range f.tags {
							if strings.Contains(tag, "@") {
								f.tags[tag] = lifecycleOldID
							}
							_ = id
						}
						f.mu.Unlock()
					}
				}
				calls := 0
				if phase != "pull" {
					f.onTag = func(source string) error {
						if source == lifecycleNewID {
							calls++
							if phase == "before-effect" || calls == 2 {
								return errors.New("synthetic tag failure")
							}
						}
						return nil
					}
				}
				out := f.wait(f.admit(intent))
				require.Equal(t, activityDTO.StatusFailed, out.Status)
				if phase == "selection-effect" {
					require.NotEmpty(t, out.ImageLifecycle.RestorePointID, "a successful Effect commit inside selection is sufficient")
					require.Len(t, lifecyclePoints(t, f), 1)
				} else {
					require.Empty(t, out.ImageLifecycle.RestorePointID)
					if mode == "scheduled" {
						require.Equal(t, &dto.RecoveryResult{Outcome: "not-needed"}, out.ImageLifecycle.Recovery)
					}
					require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
					require.Empty(t, lifecyclePoints(t, f))
				}
				require.Zero(t, f.deploys)
			})
		}
	}
}
func cancelStackRun(t *testing.T, f *lifecycleFixture) {
	t.Helper()
	var run life.Run
	require.NoError(t, f.db.First(&run).Error)
	_, err := f.runtime.Cancel(t.Context(), activity.RuntimeRunReference{Store: f.binding, RunID: run.ID}, user.Actor{ID: "operator", Username: "operator"})
	require.NoError(t, err)
}
func TestObservedStackCancellationStopsBeforeNextEffect(t *testing.T) {
	for _, phase := range []string{"before-effect", "observer", "recovery-retag", "recovery-write", "recovery-first-file", "recovery-create-barrier"} {
		t.Run(phase, func(t *testing.T) {
			f, clock := observedStackFixture(t)
			kind := "version"
			if phase == "recovery-retag" {
				kind = "rebuild"
			}
			intent := observedStackIntent(t, f, "scheduled", kind)
			recoveryTags := 0
			writesAfterCancel := 0
			switch phase {
			case "before-effect":
				f.onPull = func() { cancelStackRun(t, f) }
			case "observer":
				clock.tick = func() { cancelStackRun(t, f) }
			case "recovery-retag":
				f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
				f.onTag = func(source string) error {
					if f.deploys == 1 && source == lifecycleOldID {
						recoveryTags++
						if recoveryTags == 2 {
							cancelStackRun(t, f)
						}
					}
					return nil
				}
			case "recovery-create-barrier":
				f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
				f.service.composeCoordinator = lifecycleCreateBarrier{ComposeCoordinator: f.service.composeCoordinator, before: func() {
					if f.deploys == 1 {
						cancelStackRun(t, f)
					}
				}}
			case "recovery-write", "recovery-first-file":
				f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
				writer := lifecycleWriteFile
				t.Cleanup(func() { lifecycleWriteFile = writer })
				cancelled := false
				lifecycleWriteFile = func(ctx context.Context, root, path string, body []byte, opts acfs.WriteOptions) error {
					if cancelled {
						writesAfterCancel++
					}
					err := writer(ctx, root, path, body, opts)
					if phase == "recovery-write" && path == "/compose.yaml" && strings.Contains(string(body), "app:1.0.0") || phase == "recovery-first-file" && f.deploys == 1 {
						cancelStackRun(t, f)
						cancelled = true
					}
					return err
				}
			}
			out := f.wait(f.admit(intent))
			require.Zero(t, writesAfterCancel, "no later file effect may start after cancellation")
			require.Equal(t, activityDTO.StatusCancelled, out.Status, "%+v", out.ImageLifecycle)
			require.NotNil(t, out.ImageLifecycle.Recovery)
			if phase == "before-effect" {
				require.Equal(t, "not-needed", out.ImageLifecycle.Recovery.Outcome)
				require.Empty(t, out.ImageLifecycle.RestorePointID)
				require.Zero(t, f.deploys)
			} else {
				require.Equal(t, "interrupted", out.ImageLifecycle.Recovery.Outcome)
				require.Equal(t, 1, f.deploys, "no recovery deployment may start after cancellation")
				if phase == "recovery-retag" {
					require.Equal(t, 2, recoveryTags)
				}
				if phase == "recovery-write" {
					require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
				}
				if phase == "recovery-first-file" {
					require.Contains(t, lifecycleFile(t, f, "compose.yaml"), "app:1.1.0")
				}
			}
		})
	}
}
func TestObservedStackRawProvenanceIncludesAbsentEnv(t *testing.T) {
	f, _ := observedStackFixture(t)
	require.NoError(t, os.Remove(filepath.Join(f.project.Path, ".env")))
	f.establishBaseline()
	f.up = func(context.Context, *compose.Project) error {
		require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("SYNTHETIC_VALUE=external\n"), 0600))
		return errors.New("synthetic deploy failure")
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "version")))
	requireStackRecovery(t, out, "unavailable", "restore.files-changed")
	require.Equal(t, "SYNTHETIC_VALUE=external\n", lifecycleFile(t, f, ".env"))
}

func TestObservedStackSuccessfulRebuildRecoveryHasNoSecondReconciliation(t *testing.T) {
	f, _ := observedStackFixture(t)
	recoveryTags := 0
	f.onTag = func(source string) error {
		if f.deploys > 0 && source == lifecycleOldID {
			recoveryTags++
		}
		return nil
	}
	f.up = func(context.Context, *compose.Project) error {
		if f.deploys == 1 {
			return errors.New("synthetic deploy failure")
		}
		return nil
	}
	out := f.wait(f.admit(observedStackIntent(t, f, "scheduled", "rebuild")))
	requireStackRecovery(t, out, "restored", "")
	require.Equal(t, 2, recoveryTags, "one run pin plus one restore of the shared reference")
	require.Equal(t, lifecycleOldID, f.tags["registry.example.com/app:1.0.0"])
	require.Equal(t, 2, f.deploys)
}
