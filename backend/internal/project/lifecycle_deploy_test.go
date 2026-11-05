// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original runtime deploy points, recovery, no-refusal, ordering and protection tests.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	"github.com/florianfinn/docklet-next/types/system"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func deployPointCommand(f *lifecycleFixture, op activity.RuntimeOperation) activity.RuntimeCommand {
	return activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: op, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID, Name: new("example-stack")}, Options: activity.RuntimeOptions{Deploy: &projectDTO.DeployOptions{PullPolicy: "never"}}}
}
func runDeployPoint(t *testing.T, f *lifecycleFixture, op activity.RuntimeOperation) activityDTO.RuntimeSnapshot {
	t.Helper()
	f.normalDeploy = true
	admitted, err := f.runtime.Admit(t.Context(), deployPointCommand(f, op))
	require.NoError(t, err)
	return f.wait(admitted)
}

func TestDeployPointLastRecordedBaselineAndRecovery(t *testing.T) {
	f := newLifecycleFixture(t)
	baseline := f.establishBaseline()
	require.NoError(t, f.service.lifecycleStore().Invalidate(t.Context(), f.project.ID))
	edited := strings.Replace(lifecycleOriginal, "app:1.0.0", "app:1.1.0", 1)
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(edited), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "service.env"), []byte("SYNTHETIC_VALUE=edited\n"), 0600))
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic compose failure") }
	failed := runDeployPoint(t, f, activity.RuntimeDeployProject)
	require.Equal(t, activityDTO.StatusFailed, failed.Status)
	require.NotNil(t, failed.ImageLifecycle)
	require.NotEmpty(t, failed.ImageLifecycle.RestorePointID)
	point, saved, err := f.service.lifecycleStore().Point(t.Context(), failed.ImageLifecycle.RestorePointID)
	require.NoError(t, err)
	require.Equal(t, baseline.Files, saved.Files)
	require.Equal(t, baseline.Services, saved.Services)
	require.Equal(t, baseline.RunID, saved.RunID, "last recorded provenance survives")
	require.Equal(t, "settled", point.SourceState)
	require.False(t, point.FailedAttempt)
	var run life.Run
	require.NoError(t, f.db.First(&run, "id = ?", failed.ID).Error)
	require.True(t, run.HadEffect)
	require.True(t, run.Settled)
	f.up = nil
	f.normalDeploy = false
	restored := f.wait(f.admit(lifecycleRestore(t, f, point.ID)))
	require.Equal(t, activityDTO.StatusSuccess, restored.Status)
	require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
	require.Equal(t, "SYNTHETIC_VALUE=original\n", lifecycleFile(t, f, "service.env"))
	f.mu.Lock()
	workload := f.workloads["app"]
	f.mu.Unlock()
	require.Equal(t, lifecycleOldID, workload.ImageID)
	require.True(t, workload.Running)
}

func TestDeployPointNoRefusal(t *testing.T) {
	for _, name := range []string{"first-deploy", "identity", "registration", "multi-file", "missing-image", "prepare-failure", "pin-failure", "run-write-failure", "baseline-read-failure", "effect-write-failure", "effect-read-failure", "baseline-multi-file"} {
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			var baseline *life.Snapshot
			if name != "first-deploy" {
				baseline = f.establishBaseline()
			}
			code := "deploy.point-unavailable"
			switch name {
			case "identity":
				require.NoError(t, f.db.Model(&life.Baseline{}).Where("target_id = ?", f.project.ID).Update("target_identity", "different").Error)
			case "registration":
				require.NoError(t, f.db.Model(&life.Baseline{}).Where("target_id = ?", f.project.ID).Update("registration_id", "different").Error)
			case "baseline-multi-file":
				code = "update.image-binding-unsupported"
				for i := range baseline.Files {
					if baseline.Files[i].Path == "compose.yaml" {
						baseline.Files[i].Content = append([]byte("include:\n  - included.yaml\n"), baseline.Files[i].Content...)
					}
				}
				require.NoError(t, f.service.lifecycleStore().SaveBaseline(t.Context(), baseline))
			case "multi-file":
				code = "update.image-binding-unsupported"
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "included.yaml"), []byte("services:\n  extra:\n    image: registry.example.com/app:1.0.0\n"), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte("include:\n  - included.yaml\n"+lifecycleOriginal), 0600))
			case "missing-image":
				f.mu.Lock()
				delete(f.tags, lifecycleOldID)
				f.tags["registry.example.com/app:1.0.0"] = lifecycleNewID
				f.mu.Unlock()
			case "prepare-failure":
				code = "update.persistence-failed"
				require.NoError(t, f.db.Exec("CREATE TRIGGER deploy_point_fault BEFORE INSERT ON restore_points BEGIN SELECT RAISE(ABORT, 'synthetic write failure'); END").Error)
			case "pin-failure":
				code = "update.persistence-failed"
				f.onTag = func(string) error { return errors.New("synthetic tag failure") }
			case "run-write-failure":
				code = "update.persistence-failed"
				require.NoError(t, f.db.Exec("CREATE TRIGGER deploy_run_fault BEFORE INSERT ON image_lifecycle_runs BEGIN SELECT RAISE(ABORT, 'synthetic run failure'); END").Error)
			case "effect-read-failure":
				code = "update.persistence-failed"
				var fail atomic.Bool
				require.NoError(t, f.db.Callback().Query().Before("gorm:query").Register("deploy_effect_read_fault", func(tx *gorm.DB) {
					if tx.Statement.Table == "image_lifecycle_runs" && fail.CompareAndSwap(true, false) {
						tx.AddError(errors.New("synthetic effect marker read failure"))
					}
				}))
				f.onTag = func(string) error { fail.Store(true); return nil }
			case "effect-write-failure":
				code = "update.persistence-failed"
				require.NoError(t, f.db.Exec("CREATE TRIGGER deploy_effect_fault BEFORE UPDATE OF had_effect ON image_lifecycle_runs BEGIN SELECT RAISE(ABORT, 'synthetic effect marker failure'); END").Error)
			case "baseline-read-failure":
				code = "update.persistence-failed"
				require.NoError(t, f.db.Migrator().DropTable(&life.Baseline{}))
			}
			out := runDeployPoint(t, f, activity.RuntimeDeployProject)
			require.Equal(t, activityDTO.StatusSuccess, out.Status)
			require.NotNil(t, out.ImageLifecycle)
			require.Equal(t, code, out.ImageLifecycle.PointCode)
			require.Empty(t, out.ImageLifecycle.RestorePointID)
			require.Empty(t, lifecyclePoints(t, f))
			f.mu.Lock()
			count := f.deploys
			f.mu.Unlock()
			require.Equal(t, 1, count, "deploy must actually occur without a point")
			if name == "missing-image" || name == "pin-failure" {
				require.True(t, baseline.Services["app"].Present)
				var row life.Baseline
				require.NoError(t, f.db.First(&row, "target_id = ?", f.project.ID).Error)
				// Baseline was replaced on success; it still describes actual workload presence.
				saved, err := life.Open(row.Payload)
				require.NoError(t, err)
				require.True(t, saved.Services["app"].Present)
			}
		})
	}
}

func TestDeployPointSuccessHoldsAndDurableEvidence(t *testing.T) {
	for _, op := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
		t.Run(string(op), func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			var pointID string
			f.up = func(context.Context, *compose.Project) error {
				points := lifecyclePoints(t, f)
				require.Len(t, points, 1)
				pointID = points[0].ID
				lifecycleCode(t, f.service.DeleteImageRestorePoint(t.Context(), f.project.ID, pointID, true), "restore.in-use")
				var run life.Run
				require.NoError(t, f.db.First(&run, "point_id = ?", pointID).Error)
				require.True(t, run.HadEffect, "effect marker precedes compose up")
				return nil
			}
			out := runDeployPoint(t, f, op)
			require.Equal(t, activityDTO.StatusSuccess, out.Status)
			require.NotNil(t, out.ImageLifecycle)
			require.Equal(t, pointID, out.ImageLifecycle.RestorePointID)
			require.Empty(t, out.ImageLifecycle.BaselineCode)
			require.True(t, out.ImageLifecycle.Verified)
			held, err := f.service.lifecycleStore().Held(t.Context(), pointID)
			require.NoError(t, err)
			require.False(t, held)
			// An unprimed later read is a distinct source, after reconciliation ran.
			require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
			later, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.binding, RunID: out.ID})
			require.NoError(t, err)
			require.NotNil(t, later.ImageLifecycle)
			require.Equal(t, pointID, later.ImageLifecycle.RestorePointID)
			require.True(t, later.ImageLifecycle.Verified)
			require.NoError(t, f.service.DeleteImageRestorePoint(t.Context(), f.project.ID, pointID, true))
		})
	}
}

func TestRedeployPointPrecedesTagMovingPull(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	type observation struct {
		points []life.Point
		saved  *life.Snapshot
		pin    string
		err    error
	}
	var observations []observation
	f.onPull = func() {
		var observed observation
		observed.err = f.db.Order("created_at,id").Find(&observed.points).Error
		if observed.err == nil && len(observed.points) == 1 {
			_, observed.saved, observed.err = f.service.lifecycleStore().Point(t.Context(), observed.points[0].ID)
		}
		f.mu.Lock()
		defer f.mu.Unlock()
		if len(observed.points) == 1 {
			observed.pin = f.tags[life.Pin("points", observed.points[0].ID, "app")]
		}
		observations = append(observations, observed)
	}
	out := runDeployPoint(t, f, activity.RuntimeRedeployProject)
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	f.mu.Lock()
	observed := append([]observation(nil), observations...)
	moved, pulls := f.tags["registry.example.com/app:1.0.0"], f.pulls
	f.mu.Unlock()
	require.Len(t, observed, 1, "the actual pull records its own point state")
	require.NoError(t, observed[0].err)
	require.Len(t, observed[0].points, 1, "point exists before pull")
	require.NotNil(t, observed[0].saved)
	require.Equal(t, lifecycleOldID, observed[0].saved.Services["app"].ImageID)
	require.Equal(t, lifecycleOldID, observed[0].pin)
	require.NotNil(t, out.ImageLifecycle)
	require.Equal(t, observed[0].points[0].ID, out.ImageLifecycle.RestorePointID)
	_, saved, err := f.service.lifecycleStore().Point(t.Context(), out.ImageLifecycle.RestorePointID)
	require.NoError(t, err)
	require.Equal(t, lifecycleOldID, saved.Services["app"].ImageID)
	require.Positive(t, pulls)
	require.Equal(t, lifecycleNewID, moved)
}

func TestDeployPointSharedRetentionAndImageGuard(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	// Existing update points and deploy points share one target pool.
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	anchor := lifecyclePoints(t, f)[0]
	require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", anchor.ID).Update("last_restored_at", anchor.CreatedAt).Error)
	var oldest string
	for i := 0; i < 6; i++ {
		out := runDeployPoint(t, f, activity.RuntimeDeployProject)
		require.Equal(t, activityDTO.StatusSuccess, out.Status)
		if i == 0 {
			oldest = out.ImageLifecycle.RestorePointID
		}
	}
	require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	points := lifecyclePoints(t, f)
	require.Len(t, points, 5)
	ids := map[string]bool{}
	for _, point := range points {
		ids[point.ID] = true
		require.False(t, point.FailedAttempt)
	}
	require.True(t, ids[anchor.ID], "restore anchor survives mixed retention")
	require.False(t, ids[oldest], "oldest unanchored deploy point is evicted")
	// Remove workload use: the real guard must report point protection itself.
	f.mu.Lock()
	f.workloads = map[string]lifecycleWorkload{}
	f.mu.Unlock()
	report, err := f.service.imageService.ImageGuard().Prune(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll})
	require.NoError(t, err)
	require.Empty(t, report.ImagesDeleted)
	protected := false
	for _, item := range report.Skipped {
		if item.ID == lifecycleNewID {
			require.Equal(t, "image.restore-point-protected", item.Code)
			protected = true
		}
	}
	require.True(t, protected, "real prune keeps image owned by deploy points")
}

func TestDeployPointPartialPinsAreNeverUsable(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	tagged := 0
	f.onTag = func(string) error {
		tagged++
		if tagged == 2 {
			return errors.New("synthetic partial pin failure")
		}
		return nil
	}
	out := runDeployPoint(t, f, activity.RuntimeDeployProject)
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	require.NotNil(t, out.ImageLifecycle)
	require.Equal(t, "update.persistence-failed", out.ImageLifecycle.PointCode)
	require.Empty(t, out.ImageLifecycle.RestorePointID)
	require.Equal(t, 2, tagged, "one tag installs before the second fails")
	require.Empty(t, lifecyclePoints(t, f))
	require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	var pins int64
	require.NoError(t, f.db.Table("image_protections").Where("reason = ?", "restore-point").Count(&pins).Error)
	require.Zero(t, pins)
	f.mu.Lock()
	defer f.mu.Unlock()
	for tag := range f.tags {
		require.NotContains(t, tag, "docklet-retention.invalid/points/")
	}
}

func TestDeployPointMissingSourcePreservesOlderRecoveryPresence(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	prior := runDeployPoint(t, f, activity.RuntimeDeployProject)
	require.NotNil(t, prior.ImageLifecycle)
	pointID := prior.ImageLifecycle.RestorePointID
	require.NotEmpty(t, pointID)
	f.mu.Lock()
	for tag, id := range f.tags {
		if id == lifecycleOldID {
			delete(f.tags, tag)
		}
	}
	f.tags["registry.example.com/app:1.0.0"] = lifecycleNewID
	f.mu.Unlock()
	out := runDeployPoint(t, f, activity.RuntimeDeployProject)
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	require.Equal(t, "deploy.point-unavailable", out.ImageLifecycle.PointCode)
	point, snapshot, err := f.service.lifecycleStore().Point(t.Context(), pointID)
	require.NoError(t, err)
	require.True(t, snapshot.Services["app"].Present, "missing local image cannot change workload presence")
	beforeFile := lifecycleFile(t, f, "compose.yaml")
	f.mu.Lock()
	deploys, stops := f.deploys, len(f.stopSignals)
	f.mu.Unlock()
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: pointID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
	lifecycleCode(t, err, "restore.unavailable")
	require.Equal(t, beforeFile, lifecycleFile(t, f, "compose.yaml"))
	f.mu.Lock()
	require.Equal(t, deploys, f.deploys)
	require.Equal(t, stops, len(f.stopSignals))
	f.tags[lifecycleOldID] = lifecycleOldID
	f.mu.Unlock()
	release, err := f.service.imageService.ImageGuard().Acquire(t.Context())
	require.NoError(t, err)
	require.NoError(t, f.service.lifecyclePinPoint(t.Context(), point, snapshot))
	release()
	f.normalDeploy = false
	restored := f.wait(f.admit(lifecycleRestore(t, f, pointID)))
	require.Equal(t, activityDTO.StatusSuccess, restored.Status)
	f.mu.Lock()
	workload := f.workloads["app"]
	f.mu.Unlock()
	require.Equal(t, lifecycleOldID, workload.ImageID)
	require.True(t, workload.Running)
}
