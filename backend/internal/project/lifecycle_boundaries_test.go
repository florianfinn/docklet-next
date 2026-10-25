// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Tag/file/deploy barriers, failed rebuild and baseline capture effects.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLifecycleFinalCreateAndPostDeployIDBarriers(t *testing.T) {
	for _, stage := range []string{"before-create", "after-create"} {
		t.Run(stage, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			intent := f.intent("version")
			if stage == "before-create" {
				f.service.composeCoordinator = lifecycleCreateBarrier{ComposeCoordinator: f.service.composeCoordinator, before: func() { f.mu.Lock(); f.tags["registry.example.com/app:1.1.0"] = lifecycleOldID; f.mu.Unlock() }}
			} else {
				f.up = func(context.Context, *compose.Project) error {
					f.mu.Lock()
					f.tags["registry.example.com/app:1.1.0"] = lifecycleOldID
					f.mu.Unlock()
					return nil
				}
			}
			result := f.wait(f.admit(intent))
			require.Equal(t, activityDTO.StatusFailed, result.Status)
			require.NotNil(t, result.ImageLifecycle)
			require.Equal(t, "update.target-changed", result.ImageLifecycle.Code)
			require.False(t, result.ImageLifecycle.Verified)
			f.mu.Lock()
			defer f.mu.Unlock()
			if stage == "before-create" {
				require.Zero(t, f.deploys, "changed mapping must refuse before create")
			} else {
				require.Equal(t, 1, f.deploys)
			}
		})
	}
}
func TestLifecyclePointFailurePrecedesPullAndKeepsBaseline(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	intent := f.intent("version")
	require.NoError(t, f.db.Exec(`CREATE TRIGGER fail_point BEFORE INSERT ON restore_points BEGIN SELECT RAISE(ABORT,'synthetic point persistence failure'); END`).Error)
	result := f.wait(f.admit(intent))
	require.Equal(t, activityDTO.StatusFailed, result.Status)
	require.NotNil(t, result.ImageLifecycle)
	require.Equal(t, "update.persistence-failed", result.ImageLifecycle.Code)
	f.mu.Lock()
	require.Zero(t, f.pulls)
	require.Zero(t, f.tagWrites)
	require.Zero(t, f.deploys)
	f.mu.Unlock()
	require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
	require.Empty(t, lifecyclePoints(t, f))
	var baseline life.Baseline
	require.NoError(t, f.db.First(&baseline, "target_id = ?", f.project.ID).Error)
	require.True(t, baseline.Available)
}
func TestLifecycleCandidateProtectionSpansPullToDeployment(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	entered, release := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	f.up = func(context.Context, *compose.Project) error { close(entered); <-release; return nil }
	admitted := f.admit(f.intent("version"))
	<-entered
	var pins []guard.Protection
	require.NoError(t, f.db.Where("image_id = ? AND reason = ?", lifecycleNewID, "active-update").Find(&pins).Error)
	require.NotEmpty(t, pins)
	err := f.service.imageService.RemoveImage(t.Context(), lifecycleNewID, true, user.Actor{ID: "operator", Username: "operator"})
	lifecycleCode(t, err, "image.active-update-protected")
	close(release)
	require.Equal(t, activityDTO.StatusSuccess, f.wait(admitted).Status)
	require.NoError(t, f.db.Where("image_id = ? AND reason = ?", lifecycleNewID, "active-update").Find(&pins).Error)
	require.Empty(t, pins)
}
func TestLifecycleRestoreSharingRequiresAnActualConflictingMove(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	updated := f.wait(f.admit(f.intent("rebuild")))
	require.Equal(t, activityDTO.StatusSuccess, updated.Status)
	point := lifecyclePoints(t, f)[0]
	f.mu.Lock()
	f.workloads["peer"] = lifecycleWorkload{ID: strings.Repeat("e", 64), Project: "other-stack", Service: "app", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleNewID, Running: false}
	f.mu.Unlock()
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	request := dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true}
	_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, request)
	assert.Error(t, err)
	assert.Equal(t, "restore.shared-reference", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Len(t, lifecyclePoints(t, f), 1)
	f.mu.Lock()
	f.tags["registry.example.com/app:1.0.0"] = lifecycleOldID
	f.mu.Unlock()
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, request)
	require.NoError(t, err, "no tag move needs no shared-reference refusal")
	f.mu.Lock()
	f.tags["registry.example.com/app:1.0.0"] = lifecycleNewID
	before := f.deploys
	f.mu.Unlock()
	out := f.wait(f.admit(intent))
	f.mu.Lock()
	deploys, tag := f.deploys, f.tags["registry.example.com/app:1.0.0"]
	f.mu.Unlock()
	require.Equal(t, before, deploys)
	require.Equal(t, lifecycleNewID, tag)
	require.Equal(t, "restore.shared-reference", out.ImageLifecycle.Code)
	require.Len(t, lifecyclePoints(t, f), 1)
}
func TestLifecycleFailedRebuildReconcilesTagOrHoldsUnknown(t *testing.T) {
	for _, external := range []bool{false, true} {
		name := "reconciled"
		if external {
			name = "external-writer"
		}
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			f.up = func(context.Context, *compose.Project) error {
				if external {
					f.mu.Lock()
					f.tags["registry.example.com/app:1.0.0"] = "sha256:" + strings.Repeat("e", 64)
					f.mu.Unlock()
				}
				return errors.New("synthetic deployment failure")
			}
			result := f.wait(f.admit(f.intent("rebuild")))
			f.mu.Lock()
			tag := f.tags["registry.example.com/app:1.0.0"]
			f.mu.Unlock()
			if external {
				require.Equal(t, activityDTO.RuntimeOutcomeUnknown, result.OutcomeState)
				require.Equal(t, "sha256:"+strings.Repeat("e", 64), tag)
				var n int64
				require.NoError(t, f.db.Model(&guard.Protection{}).Where("reason = ?", "active-update").Count(&n).Error)
				require.Positive(t, n)
			} else {
				require.Equal(t, activityDTO.StatusFailed, result.Status)
				require.Equal(t, lifecycleOldID, tag)
			}
		})
	}
}
func TestLifecycleNormalDeployCapturesActualInputsOutsideImageLease(t *testing.T) {
	for _, capture := range []bool{true, false} {
		name := "captured"
		if !capture {
			name = "capture-unavailable"
		}
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			release, err := f.service.imageService.ImageGuard().Acquire(t.Context())
			require.NoError(t, err)
			defer release()
			if !capture {
				clearLifecycleKeyForTest(t)
			}
			f.normalDeploy = true
			admitted, err := f.runtime.Admit(t.Context(), activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: activity.RuntimeDeployProject, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID, Name: new("example-stack")}, Options: activity.RuntimeOptions{Deploy: &projectDTO.DeployOptions{PullPolicy: "never"}}})
			require.NoError(t, err)
			result := f.wait(admitted)
			require.Equal(t, activityDTO.StatusSuccess, result.Status, "capture cannot turn successful deployment into failure")
			require.NotNil(t, result.ImageLifecycle)
			if !capture {
				require.Equal(t, "update.baseline-unavailable", result.ImageLifecycle.BaselineCode)
			}
			run := admitted.ActivityID
			var baseline life.Baseline
			err = f.db.First(&baseline, "target_id = ?", f.project.ID).Error
			if capture {
				require.NoError(t, err)
				require.True(t, baseline.Available)
				require.Equal(t, run, baseline.SourceRunID)
				snap, err := life.Open(baseline.Payload)
				require.NoError(t, err)
				require.Equal(t, lifecycleOldID, snap.Services["app"].ImageID)
			} else {
				require.Error(t, err)
			}
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, "SYNTHETIC_VALUE=original\n", lifecycleFile(t, f, "service.env"))
		})
	}
}

func TestLifecycleObservedContainerReplacementBeforeTagHasZeroEffects(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	intent := f.intent("version")
	f.onPull = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		c := f.workloads["app"]
		c.ID = strings.Repeat("e", 64)
		f.workloads["app"] = c
	}
	out := f.wait(f.admit(intent))
	require.Equal(t, activityDTO.StatusFailed, out.Status)
	require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Zero(t, f.deploys)
	require.Equal(t, "", f.tags["registry.example.com/app:1.1.0"])
}

func TestLifecycleDependencyAndStopSignalModel(t *testing.T) {
	for _, bad := range []string{"unknown", "cycle", "valid"} {
		t.Run(bad, func(t *testing.T) {
			f := newLifecycleFixture(t)
			source := lifecycleOriginal + "  worker:\n    image: registry.example.com/app:1.0.0\n"
			label := "worker"
			if bad == "unknown" {
				label = "missing"
			}
			source = strings.Replace(source, "  app:\n", "  app:\n    labels:\n      com.getarcaneapp.arcane.depends-on: "+label+"\n      com.getarcaneapp.arcane.stop-signal: sigusr1\n", 1)
			if bad == "cycle" {
				source = strings.Replace(source, "  worker:\n", "  worker:\n    depends_on: [app]\n", 1)
			}
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
			f.mu.Lock()
			f.workloads["worker"] = lifecycleWorkload{ID: strings.Repeat("e", 64), Service: "worker", Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleOldID, Running: true}
			f.mu.Unlock()
			f.establishBaseline()
			intent := f.intent("version")
			type orderEvidence struct {
				dependency bool
				signal     string
				running    bool
				signals    []string
			}
			evidence := make(chan orderEvidence, 1)
			f.up = func(_ context.Context, model *compose.Project) error {
				_, dependency := model.Services["app"].DependsOn["worker"]
				f.mu.Lock()
				observed := orderEvidence{dependency: dependency, signal: model.Services["app"].StopSignal, running: f.workloads["app"].Running, signals: append([]string(nil), f.stopSignals...)}
				f.mu.Unlock()
				evidence <- observed
				return nil
			}
			out := f.wait(f.admit(intent))
			f.mu.Lock()
			defer f.mu.Unlock()
			if bad != "valid" {
				require.Equal(t, activityDTO.StatusFailed, out.Status)
				require.Equal(t, "update.invalid-target", out.ImageLifecycle.Code)
				require.Zero(t, f.pulls)
				require.Zero(t, f.tagWrites)
				require.Zero(t, f.deploys)
			} else {
				require.Equal(t, activityDTO.StatusSuccess, out.Status)
				observed := <-evidence
				require.True(t, observed.dependency)
				require.Equal(t, "SIGUSR1", observed.signal)
				require.Contains(t, observed.signals, "SIGUSR1")
				require.False(t, observed.running)
				require.Equal(t, activityDTO.StatusSuccess, out.Status)
			}
		})
	}
}

func TestLifecycleRestorePartialSourcePreservesAbsentAndStoppedServices(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("version"))).Status)
	original := lifecyclePoints(t, f)[0]
	partial := lifecycleFile(t, f, "compose.yaml") + "  worker:\n    image: registry.example.com/app:1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(partial), 0600))
	f.mu.Lock()
	c := f.workloads["app"]
	c.Running = false
	f.workloads["app"] = c
	f.mu.Unlock()
	restore := func(point string) {
		_, cur, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
		require.NoError(t, err)
		intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: point, ExpectedRevision: cur.SavedRevision, ReplaceConfiguration: true})
		require.NoError(t, err)
		out := f.wait(f.admit(intent))
		require.Equal(t, activityDTO.StatusSuccess, out.Status)
		require.True(t, out.ImageLifecycle.Verified)
	}
	restore(original.ID)
	points := lifecyclePoints(t, f)
	require.Len(t, points, 2)
	snap, err := life.Open(points[1].Payload)
	require.NoError(t, err)
	require.False(t, snap.Services["app"].Running)
	require.False(t, snap.Services["worker"].Present)
	f.up = func(_ context.Context, model *compose.Project) error {
		assert.NotContains(t, model.Services, "worker")
		return nil
	}
	restore(points[1].ID)
	require.Equal(t, partial, lifecycleFile(t, f, "compose.yaml"))
	f.mu.Lock()
	defer f.mu.Unlock()
	require.False(t, f.workloads["app"].Running)
	require.NotContains(t, f.workloads, "worker")
}

func TestLifecycleFileCASAndActorCannotOverwriteAnEditorChange(t *testing.T) {
	for _, which := range []string{"revision", "actor"} {
		t.Run(which, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			intent := f.intent("version")
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			desired, err := lifecycleVersionFiles(current, intent.Selections)
			require.NoError(t, err)
			actor := user.Actor{ID: "operator", Username: "operator"}
			expected := lifecycleOriginal
			if which == "revision" {
				expected += "# editor changed\n"
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(expected), 0600))
			} else {
				actor = user.SystemUser
			}
			err = f.service.lifecycleWrite(t.Context(), f.project, current, desired, actor)
			require.Equal(t, expected, lifecycleFile(t, f, "compose.yaml"))
			require.Error(t, err)
		})
	}
}
func TestLifecycleAmbiguousReferencePointRefusesBeforeAnyImageMovement(t *testing.T) {
	f := newLifecycleFixture(t)
	source := lifecycleOriginal + "  worker:\n    image: registry.example.com/app:1.0.0\n"
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	f.mu.Lock()
	f.workloads["worker"] = lifecycleWorkload{ID: strings.Repeat("e", 64), Service: "worker", Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleNewID, Running: false}
	f.mu.Unlock()
	_, snap, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	snap.RunID = uuid.NewString()
	point, err := f.service.lifecycleStore().Prepare(t.Context(), snap, snap.RunID, "", "partial-source")
	require.NoError(t, err)
	require.NoError(t, f.service.lifecyclePinPoint(t.Context(), point, snap))
	f.mu.Lock()
	beforeTags := f.tagWrites
	f.mu.Unlock()
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: snap.SavedRevision, ReplaceConfiguration: true})
	if err == nil {
		f.wait(f.admit(intent))
	}
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Equal(t, beforeTags, f.tagWrites, "an impossible point must refuse before installing more pins or moving user tags")
	require.Equal(t, lifecycleOldID, f.tags["registry.example.com/app:1.0.0"])
	require.Zero(t, f.deploys)
	require.Zero(t, f.pulls)
	lifecycleCode(t, err, "restore.unavailable")
}

type lifecycleCreateBarrier struct {
	projectDTO.ComposeCoordinator
	before func()
}

func (b lifecycleCreateBarrier) Deploy(ctx context.Context, request projectDTO.ComposeDeployment) (*compose.Project, error) {
	resolve := request.ResolveImages
	request.ResolveImages = func(ctx context.Context) (projectDTO.ComposeImageOperations, error) {
		ops, err := resolve(ctx)
		if err != nil {
			return ops, err
		}
		exists := ops.Exists
		ops.Exists = func(ctx context.Context, ref string) (bool, error) {
			ok, err := exists(ctx, ref)
			if err == nil && ok {
				b.before()
			}
			return ok, err
		}
		return ops, nil
	}
	return b.ComposeCoordinator.Deploy(ctx, request)
}

func TestLifecycleOptionalEnvAbsenceIsCapturedAndRestored(t *testing.T) {
	f := newLifecycleFixture(t)
	require.NoError(t, os.Remove(filepath.Join(f.project.Path, ".env")))
	f.establishBaseline()
	out := f.wait(f.admit(f.intent("version")))
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	point := lifecyclePoints(t, f)[0]
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("SYNTHETIC_INPUT=newer\n"), 0600))
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	restore, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
	require.NoError(t, err)
	out = f.wait(f.admit(restore))
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	require.NoFileExists(t, filepath.Join(f.project.Path, ".env"))
}

func TestLifecycleSystemImageReferenceRefusesBeforeMutation(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	intent := f.intent("version")
	f.service.imageService.ImageGuard().SystemReferences = func() []string { return []string{"registry.example.com/app:1.0.0"} }
	out := f.wait(f.admit(intent))
	f.mu.Lock()
	defer f.mu.Unlock()
	require.Zero(t, f.pulls)
	require.Zero(t, f.tagWrites)
	require.Zero(t, f.deploys)
	require.Equal(t, activityDTO.StatusFailed, out.Status)
}

func TestLifecycleSnapshotRefusesUnsafeImageReferenceWithoutEcho(t *testing.T) {
	f := newLifecycleFixture(t)
	marker := uuid.NewString()
	unsafe := "operator:" + marker + "@registry.example.com/app:1.0.0"
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(strings.Replace(lifecycleOriginal, "registry.example.com/app:1.0.0", unsafe, 1)), 0600))
	_, snapshot, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.service.LifecycleRegistration(t.Context()))
	if err == nil {
		release, e := f.service.imageService.ImageGuard().Acquire(t.Context())
		require.NoError(t, e)
		point, e := f.service.lifecycleStore().Prepare(t.Context(), snapshot, uuid.NewString(), "", "partial-source")
		if e == nil {
			e = f.service.lifecyclePinPoint(t.Context(), point, snapshot)
		}
		release()
		require.NoError(t, e)
	}
	points, listErr := f.service.ListImageRestorePoints(t.Context(), f.project.ID, false)
	require.NoError(t, listErr)
	encoded, encodeErr := json.Marshal(points)
	require.NoError(t, encodeErr)
	require.NotContains(t, string(encoded), marker, "reference metadata must never expose inline authentication")
	require.Empty(t, points)
	require.Error(t, err)
}

func TestLifecycleSnapshotMustFitAllServiceRestoreLimit(t *testing.T) {
	f := newLifecycleFixture(t)
	source := lifecycleOriginal
	for n := 0; n < 256; n++ {
		source += fmt.Sprintf("  worker%d:\n    image: registry.example.com/app:1.0.0\n", n)
		name := fmt.Sprintf("worker%d", n)
		f.workloads[name] = lifecycleWorkload{ID: fmt.Sprintf("%064x", n+1), Service: name, Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleOldID, Running: true}
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	_, snapshot, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	_, err = f.service.lifecycleStore().Prepare(t.Context(), snapshot, uuid.NewString(), "", "partial-source")
	require.Empty(t, lifecyclePoints(t, f), "do not create a point whose complete restore exceeds admission limits")
	lifecycleCode(t, err, "update.snapshot-unavailable")
}

func TestLifecycleErrorsPreserveHTTPClassification(t *testing.T) {
	for code, status := range map[string]int{"update.invalid-target": 400, "update.target-not-found": 404, "restore.not-found": 404, "update.persistence-failed": 500, "image.tag-failed": 500, "update.deploy-failed": 500, "restore.deploy-failed": 500, "image.pull-failed": 502, "update.check-unavailable": 503, "update.config-drift": 409, "restore.shared-reference": 409, "update.snapshot-unavailable": 409, "update.path-retired": 409} {
		require.Equal(t, status, imageDTO.Neutral(lifecycleFailure(code), "unexpected", 500).Status, code)
	}
}

func TestLifecycleSnapshotMustFitRestoreByteBudget(t *testing.T) {
	f := newLifecycleFixture(t)
	source := lifecycleOriginal
	ref := "registry.example.com/" + strings.Repeat("a", 210) + ":1.0.0"
	f.tags[ref] = lifecycleOldID
	for n := 0; n < 150; n++ {
		name := fmt.Sprintf("worker%d", n) + strings.Repeat("b", 210)
		source += fmt.Sprintf("  %s:\n    image: %s\n", name, ref)
		f.workloads[name] = lifecycleWorkload{ID: fmt.Sprintf("%064x", n+1), Service: name, Project: "example-stack", Reference: ref, ImageID: lifecycleOldID, Running: true}
	}
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	_, snapshot, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	_, err = f.service.lifecycleStore().Prepare(t.Context(), snapshot, uuid.NewString(), "", "partial-source")
	require.Empty(t, lifecyclePoints(t, f))
	lifecycleCode(t, err, "update.snapshot-unavailable")
}
