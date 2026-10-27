// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual saved-file, actor, point, pin and image-ID effects through runtime admission.

package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func lifecycleCode(t *testing.T, err error, code string) {
	t.Helper()
	require.Error(t, err)
	require.Equal(t, code, imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
}
func lifecycleFile(t *testing.T, f *lifecycleFixture, name string) string {
	t.Helper()
	body, err := os.ReadFile(filepath.Join(f.project.Path, name))
	require.NoError(t, err)
	return string(body)
}
func lifecyclePoints(t *testing.T, f *lifecycleFixture) []life.Point {
	t.Helper()
	var rows []life.Point
	require.NoError(t, f.db.Order("created_at,id").Find(&rows).Error)
	return rows
}
func TestLifecycleVersionRebuildAndRestoreExactInputs(t *testing.T) {
	for _, kind := range []string{"version", "rebuild"} {
		t.Run(kind, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			f.up = func(_ context.Context, model *compose.Project) error {
				points := lifecyclePoints(t, f)
				require.Len(t, points, 1)
				require.True(t, points[0].Available, "point commits before deployment")
				require.Equal(t, "never", model.Services["app"].PullPolicy)
				return nil
			}
			updated := f.wait(f.admit(f.intent(kind)))
			require.Equal(t, activityDTO.StatusSuccess, updated.Status)
			require.NotNil(t, updated.ImageLifecycle)
			require.True(t, updated.ImageLifecycle.Verified)
			expected := lifecycleOriginal
			if kind == "version" {
				expected = strings.Replace(expected, "app:1.0.0", "app:1.1.0", 1)
			}
			require.Equal(t, expected, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, "SYNTHETIC_VALUE=original\n", lifecycleFile(t, f, "service.env"))
			points := lifecyclePoints(t, f)
			require.Len(t, points, 1)
			point := points[0]
			require.NotContains(t, point.Payload, "SYNTHETIC_VALUE")
			require.NotContains(t, point.Payload, "services:")
			saved, err := life.Open(point.Payload)
			require.NoError(t, err)
			require.Equal(t, lifecycleOldID, saved.Services["app"].ImageID)
			var audits []event.Event
			require.NoError(t, f.db.Where("type = ?", event.EventTypeProjectUpdate).Find(&audits).Error)
			if kind == "version" {
				require.NotEmpty(t, audits)
				require.Equal(t, "operator", *audits[0].UserID)
			}
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(expected+"# undeployed operator edit\n"), 0600))
			require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("STACK_INPUT=newer\n"), 0600))
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			request := dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: current.SavedRevision}
			_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, request)
			lifecycleCode(t, err, "update.config-drift")
			request.ReplaceConfiguration = true
			restore, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, request)
			require.NoError(t, err)
			f.up = nil
			restored := f.wait(f.admit(restore))
			require.Equal(t, activityDTO.StatusSuccess, restored.Status)
			require.True(t, restored.ImageLifecycle.Verified)
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, "STACK_INPUT=original\n", lifecycleFile(t, f, ".env"))
			points = lifecyclePoints(t, f)
			require.Len(t, points, 2)
			require.Equal(t, "partial-source", points[1].SourceState)
			observed, err := life.Open(points[1].Payload)
			require.NoError(t, err)
			require.Equal(t, lifecycleNewID, observed.Services["app"].ImageID)
			require.Contains(t, string(observed.Files[0].Content)+string(observed.Files[1].Content)+string(observed.Files[2].Content), "undeployed operator edit")
			status, err := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			require.True(t, status.Available)
		})
	}
}
func TestLifecycleUpdateRefusalsHaveZeroWorkloadAndFileEffects(t *testing.T) {
	for _, name := range []string{"no-baseline", "drift", "nonliteral", "busy", "missing-key", "actor"} {
		t.Run(name, func(t *testing.T) {
			f := newLifecycleFixture(t)
			if name == "nonliteral" {
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(strings.Replace(lifecycleOriginal, "app:1.0.0", "app:${TAG}", 1)), 0600))
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, ".env"), []byte("TAG=1.0.0\n"), 0600))
			}
			if name != "no-baseline" {
				f.establishBaseline()
			}
			before := lifecycleFile(t, f, "compose.yaml")
			if name == "drift" {
				before += "# changed\n"
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(before), 0600))
			}
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			if name == "no-baseline" || name == "drift" {
				_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
				code := "update.baseline-unavailable"
				if name == "drift" {
					code = "update.config-drift"
				}
				lifecycleCode(t, err, code)
			} else {
				intent := f.intent("version")
				switch name {
				case "busy":
					release, err := f.service.imageService.ImageGuard().Acquire(t.Context())
					require.NoError(t, err)
					defer release()
				case "missing-key":
					clearLifecycleKeyForTest(t)
				}
				if name == "actor" {
					_, err = f.service.ExecuteImageLifecycle(t.Context(), f.project.ID, uuid.NewString(), f.registration, intent, user.SystemUser)
					lifecycleCode(t, err, "update.invalid-target")
				} else {
					result := f.wait(f.admit(intent))
					assert.Equal(t, activityDTO.StatusFailed, result.Status)
					require.NotNil(t, result.ImageLifecycle)
					code := map[string]string{"busy": "image.busy", "missing-key": "update.snapshot-unavailable", "nonliteral": "update.image-binding-unsupported"}[name]
					assert.Equal(t, code, result.ImageLifecycle.Code)
				}
			}
			require.Equal(t, before, lifecycleFile(t, f, "compose.yaml"))
			f.mu.Lock()
			require.Zero(t, f.deploys)
			require.Zero(t, f.pulls)
			require.Zero(t, f.tagWrites)
			f.mu.Unlock()
			require.Empty(t, lifecyclePoints(t, f))
		})
	}
}
func TestLifecyclePullBoundaryRechecksSavedFilesBeforeTags(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	intent := f.intent("version")
	f.onPull = func() {
		require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(lifecycleOriginal+"# editor raced\n"), 0600))
	}
	result := f.wait(f.admit(intent))
	assert.Equal(t, activityDTO.StatusFailed, result.Status)
	require.NotNil(t, result.ImageLifecycle)
	require.Equal(t, "update.target-changed", result.ImageLifecycle.Code)
	require.Equal(t, lifecycleOriginal+"# editor raced\n", lifecycleFile(t, f, "compose.yaml"))
	f.mu.Lock()
	require.Zero(t, f.deploys)
	_, moved := f.tags["registry.example.com/app:1.1.0"]
	f.mu.Unlock()
	require.False(t, moved, "racing editor write refuses before source tag assignment")
	require.Empty(t, lifecyclePoints(t, f), "no-effect failure drops its preparing point")
}
func TestLifecycleFailedVersionCanRestoreWithoutSuccessfulDeploy(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	f.up = func(context.Context, *compose.Project) error {
		f.mu.Lock()
		c := f.workloads["app"]
		c.ImageID = lifecycleNewID
		c.Reference = "registry.example.com/app:1.1.0"
		f.workloads["app"] = c
		f.mu.Unlock()
		return errors.New("synthetic partial deploy")
	}
	failed := f.wait(f.admit(f.intent("version")))
	require.Equal(t, activityDTO.StatusFailed, failed.Status)
	require.NotNil(t, failed.ImageLifecycle)
	require.Equal(t, "update.deploy-failed", failed.ImageLifecycle.Code)
	require.Contains(t, lifecycleFile(t, f, "compose.yaml"), "app:1.1.0")
	points := lifecyclePoints(t, f)
	require.Len(t, points, 1)
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	_, _, err = f.service.lifecycleStore().Baseline(t.Context(), current)
	lifecycleCode(t, err, "update.baseline-unavailable")
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: points[0].ID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
	require.NoError(t, err)
	f.up = nil
	restored := f.wait(f.admit(intent))
	require.Equal(t, activityDTO.StatusSuccess, restored.Status)
	require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
	points = lifecyclePoints(t, f)
	require.Len(t, points, 2)
	require.Equal(t, "partial-source", points[1].SourceState)
	captured, err := life.Open(points[1].Payload)
	require.NoError(t, err)
	require.Equal(t, lifecycleNewID, captured.Services["app"].ImageID)
}
func TestLifecycleRestoreDuringUpdateKeepsOneOwner(t *testing.T) {
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
	select {
	case <-entered:
	case <-t.Context().Done():
		t.Fatal("deployment barrier not reached")
	}
	points := lifecyclePoints(t, f)
	require.Len(t, points, 1)
	other := activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: activity.RuntimeRestartProject, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID, Name: new("example-stack")}}
	_, err := f.runtime.Admit(t.Context(), other)
	var busy *activity.GateConflict
	require.ErrorAs(t, err, &busy)
	require.Equal(t, admitted.ActivityID, busy.RunID)
	err = f.service.DeleteImageRestorePoint(t.Context(), f.project.ID, points[0].ID, true)
	lifecycleCode(t, err, "restore.in-use")
	close(release)
	require.Equal(t, activityDTO.StatusSuccess, f.wait(admitted).Status)
}
func TestLifecyclePointPinsStayUntilLastOwner(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	for i := 0; i < 6; i++ {
		intent := f.intent("rebuild")
		expected := "sha256:" + strings.Repeat(fmt.Sprint(i+1), 64)
		intent.Selections[0].ImageID = expected
		intent.Selections[0].Digest = expected
		f.onPull = func() {
			f.mu.Lock()
			defer f.mu.Unlock()
			f.tags["registry.example.com/app@"+expected] = expected
			f.tags[expected] = expected
		}
		result := f.wait(f.admit(intent))
		require.Equal(t, activityDTO.StatusSuccess, result.Status)
	}
	_ = f.service.ReconcileImageLifecycle(t.Context())
	points := lifecyclePoints(t, f)
	require.Len(t, points, 5)
	require.NoError(t, f.db.Delete(&Project{}, "id = ?", f.project.ID).Error)
	_, err := f.service.ListImageRestorePoints(t.Context(), f.project.ID, false)
	require.Error(t, err)
	orphaned, err := f.service.ListImageRestorePoints(t.Context(), "", true)
	require.NoError(t, err)
	require.Len(t, orphaned, 5)
	require.True(t, orphaned[0].Orphaned)
	require.NoError(t, f.service.DeleteImageRestorePoint(t.Context(), "", points[0].ID, true))
	var holds []guard.Protection
	require.NoError(t, f.db.Where("owner_id = ?", points[0].ID).Find(&holds).Error)
	require.Empty(t, holds)
	require.NoError(t, f.db.Where("reason = ?", "restore-point").Find(&holds).Error)
	require.NotEmpty(t, holds, "other points keep the shared immutable image protected")
}
