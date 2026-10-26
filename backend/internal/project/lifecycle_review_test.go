// Source: docklet-next/docs/design/images-updates.md
// Revision: 48915fde5d95ed00c757116410f76617e23e8a50
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Configuration-only eligibility, recovery, retention and pulled-ID regression coverage.

package project

import (
	"context"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func lifecycleNoCandidate(f *lifecycleFixture) {
	f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
		return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {UpdateInfo: &imageDTO.UpdateInfo{}}}}, nil
	})
}
func lifecycleRestore(t *testing.T, f *lifecycleFixture, id string) *dto.LifecycleIntent {
	t.Helper()
	_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
	require.NoError(t, err)
	intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{PointID: id, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
	require.NoError(t, err)
	return intent
}

func TestLifecycleConfigurationIgnoresRuntimeShape(t *testing.T) {
	for _, shape := range []string{"stopped", "exited-init", "absent-init", "replica", "one-off", "one-off-only", "orphan", "profile-disabled", "replacement-same-image"} {
		t.Run(shape, func(t *testing.T) {
			f := newLifecycleFixture(t)
			if shape == "profile-disabled" {
				source := lifecycleOriginal + "  worker:\n    image: registry.example.com/app:1.0.0\n    profiles: [auxiliary]\n"
				require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
			}
			f.establishBaseline()
			lifecycleNoCandidate(f)
			f.mu.Lock()
			app := f.workloads["app"]
			switch shape {
			case "stopped", "exited-init":
				app.Running = false
				f.workloads["app"] = app
			case "absent-init":
				delete(f.workloads, "app")
			case "replacement-same-image":
				app.ID = strings.Repeat("1", 64)
				f.workloads["app"] = app
			default:
				extra := app
				extra.ID = strings.Repeat("1", 64)
				if shape == "orphan" || shape == "profile-disabled" {
					extra.Service = "worker"
				}
				if strings.HasPrefix(shape, "one-off") {
					extra.OneOff = true
					extra.ImageID = lifecycleNewID
				}
				f.workloads["extra"] = extra
				if shape == "one-off-only" {
					delete(f.workloads, "app")
				}
			}
			f.mu.Unlock()
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			status, err := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			require.True(t, status.Available, "runtime shape must not invalidate deployed configuration")
			_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
			require.NoError(t, err, "manual update remains admissible")
			out := f.wait(f.admit(f.intent("rebuild")))
			require.Equal(t, activityDTO.StatusSuccess, out.Status, "result=%+v", out.ImageLifecycle)
			points := lifecyclePoints(t, f)
			require.Len(t, points, 1)
			_, source, err := f.service.lifecycleStore().Point(t.Context(), points[0].ID)
			require.NoError(t, err)
			for id, img := range source.Containers {
				if _, configured := source.Services[img.Service]; !configured || img.OneOff {
					continue
				}
				var protected int64
				require.NoError(t, f.db.Table("image_protections").Where("image_id = ? AND reason = ? AND owner_id = ?", img.ImageID, "restore-point", points[0].ID).Count(&protected).Error)
				require.Positive(t, protected, "every observed source image must retain point protection")
				f.mu.Lock()
				pin := f.tags[life.Pin("points", points[0].ID, "container/"+id)]
				f.mu.Unlock()
				require.Equal(t, img.ImageID, pin, "runtime evidence must have a verified retention pin")
			}
			if shape == "orphan" || shape == "replica" || shape == "one-off" || shape == "profile-disabled" {
				require.Len(t, source.Containers, 2, "partial-source evidence includes every observed container")
			}
			restored := f.wait(f.admit(lifecycleRestore(t, f, points[0].ID)))
			require.Equal(t, activityDTO.StatusSuccess, restored.Status)
			if shape == "orphan" || strings.HasPrefix(shape, "one-off") || shape == "profile-disabled" {
				f.mu.Lock()
				_, exists := f.workloads["extra"]
				f.mu.Unlock()
				require.True(t, exists, "unrelated runtime containers are not configuration targets")
			}
		})
	}
}

func TestLifecycleRestoreIgnoresExtraCurrentContainersAndInitExit(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	points := lifecyclePoints(t, f)
	require.NotEmpty(t, points)
	f.mu.Lock()
	extra := f.workloads["app"]
	extra.ID = strings.Repeat("1", 64)
	extra.Service = "removed-worker"
	f.workloads["orphan"] = extra
	f.mu.Unlock()
	f.onList = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.deploys >= 2 {
			app := f.workloads["app"]
			app.Running = false
			f.workloads["app"] = app
		}
	}
	out := f.wait(f.admit(lifecycleRestore(t, f, points[0].ID)))
	require.Equal(t, activityDTO.StatusSuccess, out.Status, "a complete stored point must recover despite current extras and an init exit")
	require.True(t, out.ImageLifecycle.Verified)
}

func TestLifecycleConfigurationStillRefusesChangedImageBindingAndConfigHash(t *testing.T) {
	for _, change := range []string{"image", "config-hash"} {
		t.Run(change, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			lifecycleNoCandidate(f)
			f.mu.Lock()
			app := f.workloads["app"]
			if change == "image" {
				app.ImageID = lifecycleNewID
			} else {
				app.ConfigHash = "different-rendered-config"
			}
			f.workloads["app"] = app
			f.mu.Unlock()
			_, current, err := f.service.lifecycleCurrent(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			_, err = f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision})
			lifecycleCode(t, err, "update.baseline-unavailable")
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Zero(t, f.pulls)
			require.Zero(t, f.tagWrites)
			require.Zero(t, f.deploys)
		})
	}
}

func TestLifecyclePointListDoesNotReconcileOrMutateTags(t *testing.T) {
	f := newLifecycleFixture(t)
	source := f.establishBaseline()
	point, err := f.service.lifecycleStore().Prepare(t.Context(), source, uuid.NewString(), "", "settled")
	require.NoError(t, err)
	require.NoError(t, f.service.lifecyclePinPoint(t.Context(), point, source))
	require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
	f.mu.Lock()
	before := life.Hash(f.tags)
	f.mu.Unlock()
	rows, err := f.service.ListImageRestorePoints(t.Context(), f.project.ID, false)
	require.NoError(t, err)
	require.Len(t, rows, 1)
	var count int64
	require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", point.ID).Count(&count).Error)
	require.EqualValues(t, 1, count)
	f.mu.Lock()
	after := life.Hash(f.tags)
	f.mu.Unlock()
	require.Equal(t, before, after, "GET may not remove retained tags")
	require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
	require.Empty(t, lifecyclePoints(t, f), "settlement cleanup drops a no-effect point on its write path")
}

func TestLifecyclePostPullImageIDMismatchHasNoBindingOrFileEffects(t *testing.T) {
	for _, identity := range []string{"wrong-config-id", "manifest-id-store"} {
		t.Run(identity, func(t *testing.T) {
			f := newLifecycleFixture(t)
			f.establishBaseline()
			require.NoError(t, f.db.Exec("CREATE TABLE test_effect_markers(run_id TEXT NOT NULL)").Error)
			require.NoError(t, f.db.Exec("CREATE TRIGGER test_record_effect AFTER UPDATE OF had_effect ON image_lifecycle_runs WHEN NEW.had_effect = 1 BEGIN INSERT INTO test_effect_markers(run_id) VALUES(NEW.id); END").Error)
			original := lifecycleFile(t, f, "compose.yaml")
			tagsAtPull := 0
			f.onPull = func() {
				f.mu.Lock()
				defer f.mu.Unlock()
				tagsAtPull = f.tagWrites
				id := lifecycleOldID
				if identity == "manifest-id-store" {
					id = lifecycleDigest
				}
				f.tags["registry.example.com/app@"+lifecycleDigest] = id
			}
			out := f.wait(f.admit(f.intent("version")))
			f.mu.Lock()
			after := f.tagWrites
			deploys := f.deploys
			f.mu.Unlock()
			require.Equal(t, tagsAtPull, after, "the ID guard must run before candidate/user tag writes")
			var effects int64
			require.NoError(t, f.db.Table("test_effect_markers").Count(&effects).Error)
			require.Zero(t, effects, "a rejected pulled ID must never persist HadEffect")
			require.Zero(t, deploys)
			require.Equal(t, original, lifecycleFile(t, f, "compose.yaml"))
			require.Equal(t, activityDTO.StatusFailed, out.Status)
			code := "update.target-changed"

			require.Equal(t, code, out.ImageLifecycle.Code)
			status, err := f.service.ImageBaselineStatus(t.Context(), f.project.ID, f.registration)
			require.NoError(t, err)
			require.True(t, status.Available)
			require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
			require.Empty(t, lifecyclePoints(t, f), "no-effect failures drop their point")
		})
	}
}

func TestLifecycleVersionReferencePreservesLiteralRepository(t *testing.T) {
	for _, source := range []string{"nginx:1.27", "library/nginx:1.27", "docker.io/library/nginx:1.27", "registry.example.com:5443/team/nginx:1.27"} {
		t.Run(source, func(t *testing.T) {
			selected, err := lifecycleVersionReference(source, "1.28")
			require.NoError(t, err)
			require.Equal(t, strings.TrimSuffix(source, "1.27")+"1.28", selected)
		})
	}
}

func TestLifecycleVersionUpdateWritesTheUsersLiteralReference(t *testing.T) {
	f := newLifecycleFixture(t)
	original := strings.ReplaceAll(lifecycleOriginal, "registry.example.com/app:1.0.0", "nginx:1.27")
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(original), 0600))
	f.mu.Lock()
	f.tags["docker.io/library/nginx:1.27"] = lifecycleOldID
	app := f.workloads["app"]
	app.Reference = "nginx:1.27"
	f.workloads["app"] = app
	f.mu.Unlock()
	f.establishBaseline()
	intent := f.intent("version")
	selected, err := lifecycleVersionReference("nginx:1.27", "1.28")
	require.NoError(t, err)
	intent.Selections[0].SourceReference = "nginx:1.27"
	intent.Selections[0].Reference = selected
	out := f.wait(f.admit(intent))
	require.Equal(t, activityDTO.StatusSuccess, out.Status, "result=%+v", out.ImageLifecycle)
	contents := lifecycleFile(t, f, "compose.yaml")
	require.Contains(t, contents, "image: nginx:1.28 # preserve comment")
	require.NotContains(t, contents, "docker.io/library/")
}

func TestLifecycleRestoreAbsentServiceLeavesAnUnrelatedCurrentOrphan(t *testing.T) {
	f := newLifecycleFixture(t)
	f.mu.Lock()
	delete(f.workloads, "app")
	f.mu.Unlock()
	source := f.establishBaseline()
	point, err := f.service.lifecycleStore().Prepare(t.Context(), source, uuid.NewString(), "", "partial-source")
	require.NoError(t, err)
	require.NoError(t, f.service.lifecyclePinPoint(t.Context(), point, source))
	require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Updates(map[string]any{"settled": true, "had_effect": true}).Error)
	current := "services:\n  worker:\n    image: registry.example.com/app:1.0.0\n    stop_signal: SIGUSR2\n"
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(current), 0600))
	f.mu.Lock()
	f.workloads["orphan"] = lifecycleWorkload{ID: strings.Repeat("1", 64), Service: "app", Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleNewID, Running: true}
	f.workloads["worker"] = lifecycleWorkload{ID: strings.Repeat("2", 64), Service: "worker", Project: "example-stack", Reference: "registry.example.com/app:1.0.0", ImageID: lifecycleOldID, Running: true}
	f.mu.Unlock()
	out := f.wait(f.admit(lifecycleRestore(t, f, point.ID)))
	require.Equal(t, activityDTO.StatusSuccess, out.Status, "result=%+v", out.ImageLifecycle)
	f.mu.Lock()
	_, orphan := f.workloads["orphan"]
	_, worker := f.workloads["worker"]
	f.mu.Unlock()
	require.True(t, orphan, "unrelated current orphan is not a configuration target")
	require.False(t, worker, "a captured service of the replaced configuration is removed")
	f.mu.Lock()
	signals := append([]string(nil), f.stopSignals...)
	f.mu.Unlock()
	require.Contains(t, signals, "SIGUSR2", "removed configuration targets retain their source stop signal")
}

func TestLifecyclePostDeployVerifiesEveryAffectedReplica(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	f.mu.Lock()
	extra := f.workloads["app"]
	extra.ID = strings.Repeat("1", 64)
	f.workloads["replica"] = extra
	f.mu.Unlock()
	f.onList = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		if f.deploys > 0 {
			extra := f.workloads["replica"]
			extra.ImageID = lifecycleOldID
			f.workloads["replica"] = extra
		}
	}
	out := f.wait(f.admit(f.intent("rebuild")))
	require.Equal(t, activityDTO.StatusFailed, out.Status, "an unverified replica must not be reported successful")
	require.Equal(t, "update.target-changed", out.ImageLifecycle.Code)
	require.NotEmpty(t, lifecyclePoints(t, f), "partial effects retain their recovery point")
}

func TestLifecycleRunningTransitionDuringPullIsNotDrift(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	f.onPull = func() {
		f.mu.Lock()
		defer f.mu.Unlock()
		app := f.workloads["app"]
		app.Running = false
		f.workloads["app"] = app
	}
	out := f.wait(f.admit(f.intent("rebuild")))
	require.Equal(t, activityDTO.StatusSuccess, out.Status, "a state transition without a configuration or identity change must not refuse the run")
	require.True(t, out.ImageLifecycle.Verified)
}
