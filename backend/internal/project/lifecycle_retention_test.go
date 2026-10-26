// Source: docklet-next/docs/design/images-updates.md
// Revision: 727498b22e03475aa5b4e5325727ff182e8cebdd
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: M6 recovery anchors, failed-state replacement and R2 admission regressions.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestLifecycleFailedRestoresPreserveTargetAndBaseline(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.Len(t, points, 1)
	target := points[0].ID
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
	baselinePoint := ""
	for attempt := 0; attempt < 5; attempt++ {
		out := f.wait(f.admit(lifecycleRestore(t, f, target)))
		require.Equal(t, activityDTO.StatusFailed, out.Status)
		require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
		if attempt == 0 {
			baselinePoint = out.ImageLifecycle.RestorePointID
		}
		var selected, baseline life.Point
		require.NoError(t, f.db.First(&selected, "id = ?", target).Error, "restore target P must survive every failed retry")
		require.NotNil(t, selected.LastRestoredAt)
		require.NoError(t, f.db.First(&baseline, "id = ?", baselinePoint).Error, "the latest healthy source must survive failures")
		require.Equal(t, "settled", baseline.SourceState)
		points = lifecyclePoints(t, f)
		failed := 0
		for _, p := range points {
			if p.SourceState == "partial-source" {
				failed++
			}
		}
		require.LessOrEqual(t, failed, 1, "failed-state attempts replace each other")
		require.LessOrEqual(t, len(points), 3)
		for _, id := range []string{target, baselinePoint} {
			point, snapshot, err := f.service.lifecycleStore().Point(t.Context(), id)
			require.NoError(t, err)
			require.NoError(t, f.service.verifyLifecyclePoint(t.Context(), point, snapshot), "both recovery anchors retain verified pins")
		}
	}
	// The reviewer's sixth restore must still find P and actually recover.
	f.up = nil
	out := f.wait(f.admit(lifecycleRestore(t, f, target)))
	require.Equal(t, activityDTO.StatusSuccess, out.Status)
	require.True(t, out.ImageLifecycle.Verified)
}

func TestLifecycleSixthRestoreStillFindsOriginalPoint(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	target := points[0].ID
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
	for attempt := 0; attempt < 5; attempt++ {
		out := f.wait(f.admit(lifecycleRestore(t, f, target)))
		require.Equal(t, activityDTO.StatusFailed, out.Status)
		require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	}
	f.up = nil
	out := f.wait(f.admit(lifecycleRestore(t, f, target)))
	require.Equal(t, activityDTO.StatusSuccess, out.Status, "the sixth restore still resolves P")
}

func TestLifecycleFailedSlotReplacementReleasesOnlySupersededPins(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	target := points[0].ID
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
	previous := ""
	for attempt := 0; attempt < 4; attempt++ {
		out := f.wait(f.admit(lifecycleRestore(t, f, target)))
		require.Equal(t, activityDTO.StatusFailed, out.Status)
		require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
		var point life.Point
		require.NoError(t, f.db.First(&point, "id = ?", out.ImageLifecycle.RestorePointID).Error)
		if point.SourceState != "partial-source" {
			continue
		}
		if previous != "" {
			var n int64
			require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", previous).Count(&n).Error)
			require.Zero(t, n)
			require.NoError(t, f.db.Table("image_protections").Where("reason = ? AND owner_id = ?", "restore-point", previous).Count(&n).Error)
			require.Zero(t, n)
			prefix := life.Pin("points", previous, "app")
			prefix = prefix[:strings.LastIndex(prefix, "/")+1]
			f.mu.Lock()
			remaining := false
			for tag := range f.tags {
				remaining = remaining || strings.HasPrefix(tag, prefix)
			}
			f.mu.Unlock()
			require.False(t, remaining, "all superseded owner tags are removed")
		}
		previous = point.ID
	}
	require.NotEmpty(t, previous)
	point, snapshot, err := f.service.lifecycleStore().Point(t.Context(), previous)
	require.NoError(t, err)
	require.NoError(t, f.service.verifyLifecyclePoint(t.Context(), point, snapshot))
}

func TestLifecycleRestoreUseIsMonotonicAndSurvivesHistoryCleanup(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	target := points[0].ID
	future := time.Date(2200, 1, 1, 0, 0, 0, 0, time.UTC)
	require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", target).Update("last_restored_at", future).Error)
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(lifecycleRestore(t, f, target))).Status)
	var selected life.Point
	require.NoError(t, f.db.First(&selected, "id = ?", target).Error)
	require.NotNil(t, selected.LastRestoredAt)
	require.True(t, selected.LastRestoredAt.After(future), "a clock regression cannot restore an older retention order")
	require.NoError(t, f.db.Exec("DELETE FROM runtime_image_intents").Error)
	require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	require.NoError(t, f.db.First(&selected, "id = ?", target).Error)
	_, snapshot, err := f.service.lifecycleStore().Point(t.Context(), target)
	require.NoError(t, err)
	require.NoError(t, f.service.verifyLifecyclePoint(t.Context(), selected, snapshot))
}

func TestLifecycleSuccessfulPartialSourcesDoNotUseFailedSlot(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	target := points[0].ID
	var successful []string
	for cycle := 0; cycle < 2; cycle++ {
		f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic deploy failure") }
		for attempt := 0; attempt < 2; attempt++ {
			require.Equal(t, activityDTO.StatusFailed, f.wait(f.admit(lifecycleRestore(t, f, target))).Status)
		}
		f.up = nil
		out := f.wait(f.admit(lifecycleRestore(t, f, target)))
		require.Equal(t, activityDTO.StatusSuccess, out.Status)
		successful = append(successful, out.ImageLifecycle.RestorePointID)
	}
	require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	for _, id := range successful {
		var point life.Point
		require.NoError(t, f.db.First(&point, "id = ?", id).Error, "successful pre-restore snapshots remain ordinary history")
		require.Equal(t, "partial-source", point.SourceState)
		require.False(t, point.FailedAttempt)
	}
	failed := 0
	for _, point := range lifecyclePoints(t, f) {
		if point.FailedAttempt {
			failed++
		}
	}
	require.Equal(t, 1, failed)
}

func TestLifecycleUninspectableUnrelatedImagesDoNotBlockRecovery(t *testing.T) {
	for _, kind := range []string{"orphan", "one-off", "disabled-profile"} {
		t.Run(kind, func(t *testing.T) {
			f := newLifecycleFixture(t)
			if kind == "disabled-profile" {
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(lifecycleOriginal+"  worker:\n    image: registry.example.com/gone:1\n    profiles: [auxiliary]\n"), 0600))
			}
			f.establishBaseline()
			require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
			points := lifecyclePoints(t, f)
			require.NotEmpty(t, points)
			target := points[0].ID
			missing := "sha256:" + strings.Repeat("e", 64)
			extra := lifecycleWorkload{ID: strings.Repeat("f", 64), Service: "worker", Project: "example-stack", Reference: "registry.example.com/gone:1", ImageID: missing}
			if kind == "one-off" {
				extra.Service = "app"
				extra.OneOff = true
			}
			f.mu.Lock()
			f.workloads["unrelated"] = extra
			f.mu.Unlock()
			var inspected atomic.Int32
			f.onInspect = func(ref string) {
				if ref == missing {
					inspected.Add(1)
				}
			}
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			lifecycleNoCandidate(f)
			_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
			require.NoError(t, err)
			updated := f.wait(f.admit(f.intent("rebuild")))
			require.Equal(t, activityDTO.StatusFailed, updated.Status)
			require.Equal(t, "job.skipped", updated.ImageLifecycle.Code)
			restored := f.wait(f.admit(lifecycleRestore(t, f, target)))
			require.Equal(t, activityDTO.StatusSuccess, restored.Status)
			require.Zero(t, inspected.Load(), "unrelated image metadata is not required at any phase")
			f.mu.Lock()
			kept := f.workloads["unrelated"]
			f.mu.Unlock()
			require.Equal(t, extra, kept)
			var pins int64
			require.NoError(t, f.db.Table("image_protections").Where("image_id = ?", missing).Count(&pins).Error)
			require.Zero(t, pins)
		})
	}
}

func TestLifecycleMissingConfiguredImageRefusesPreflight(t *testing.T) {
	for _, operation := range []string{"update", "restore"} {
		t.Run(operation, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			request := dto.LifecycleRequest{ExpectedRevision: current.SavedRevision}
			if operation == "restore" {
				points := lifecyclePoints(t, f)
				require.NotEmpty(t, points)
				request.PointID = points[0].ID
				request.ReplaceConfiguration = true
			}
			lifecycleNoCandidate(f)
			f.mu.Lock()
			app := f.workloads["app"]
			app.ImageID = "sha256:" + strings.Repeat("e", 64)
			f.workloads["app"] = app
			beforeTags := life.Hash(f.tags)
			pulls, writes, deploys, stops := f.pulls, f.tagWrites, f.deploys, len(f.stopSignals)
			f.mu.Unlock()
			beforeFile := lifecycleFile(t, f, "compose.yaml")
			beforePoints := lifecyclePoints(t, f)
			var beforeRuns int64
			require.NoError(t, f.db.Model(&life.Run{}).Count(&beforeRuns).Error)

			intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, request)
			lifecycleCode(t, err, "update.snapshot-unavailable")
			require.Equal(t, 409, imageDTO.Neutral(err, "unexpected", 500).Status)
			require.Nil(t, intent)
			f.mu.Lock()
			afterTags := life.Hash(f.tags)
			afterPulls, afterWrites, afterDeploys, afterStops := f.pulls, f.tagWrites, f.deploys, len(f.stopSignals)
			f.mu.Unlock()
			require.Equal(t, beforeTags, afterTags)
			require.Equal(t, pulls, afterPulls)
			require.Equal(t, writes, afterWrites)
			require.Equal(t, deploys, afterDeploys)
			require.Equal(t, stops, afterStops)
			require.Equal(t, beforeFile, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, beforePoints, lifecyclePoints(t, f))
			var afterRuns int64
			require.NoError(t, f.db.Model(&life.Run{}).Count(&afterRuns).Error)
			require.Equal(t, beforeRuns, afterRuns)
		})
	}
}

func TestLifecycleRestorePreflightRejectsIncompletePointWithoutEffects(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	target := points[0].ID
	point, snapshot, err := f.service.lifecycleStore().Point(t.Context(), target)
	require.NoError(t, err)
	f.mu.Lock()
	removed := 0
	for key, img := range snapshot.Images() {
		if img.Present {
			tag := life.Pin("points", point.ID, key)
			if _, ok := f.tags[tag]; ok {
				delete(f.tags, tag)
				removed++
			}
		}
	}
	beforeTags := life.Hash(f.tags)
	pulls, writes, deploys, stops := f.pulls, f.tagWrites, f.deploys, len(f.stopSignals)
	f.mu.Unlock()
	require.Positive(t, removed)
	beforeFile := lifecycleFile(t, f, "compose.yaml")
	var beforePoints, beforeRuns int64
	require.NoError(t, f.db.Model(&life.Point{}).Count(&beforePoints).Error)
	require.NoError(t, f.db.Model(&life.Run{}).Count(&beforeRuns).Error)
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: target, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
	lifecycleCode(t, err, "restore.unavailable")
	require.Nil(t, intent)
	f.mu.Lock()
	afterTags := life.Hash(f.tags)
	afterPulls, afterWrites, afterDeploys, afterStops := f.pulls, f.tagWrites, f.deploys, len(f.stopSignals)
	f.mu.Unlock()
	require.Equal(t, beforeTags, afterTags)
	require.Equal(t, pulls, afterPulls)
	require.Equal(t, writes, afterWrites)
	require.Equal(t, deploys, afterDeploys)
	require.Equal(t, stops, afterStops)
	require.Equal(t, beforeFile, lifecycleFile(t, f, "compose.yaml"))
	var afterPoints, afterRuns int64
	require.NoError(t, f.db.Model(&life.Point{}).Count(&afterPoints).Error)
	require.NoError(t, f.db.Model(&life.Run{}).Count(&afterRuns).Error)
	require.Equal(t, beforePoints, afterPoints)
	require.Equal(t, beforeRuns, afterRuns)
}
