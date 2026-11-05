// Source: docklet-next/issues/343
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original service-scope preflight, whole-stack recovery and exclusion proofs.

package project

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

const serviceScopeSource = `services:
  app:
    image: registry.example.com/app:1.0.0
    network_mode: none
    depends_on: [stopped]
  running:
    image: registry.example.com/side:1.0.0
    network_mode: none
    labels:
      com.getarcaneapp.arcane.stop-signal: SIGUSR2
  stopped:
    image: registry.example.com/side:1.0.0
    network_mode: none
`

func serviceScopeFixture(t *testing.T, source string) *lifecycleFixture {
	t.Helper()
	f := newLifecycleFixture(t)
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	f.tags["registry.example.com/side:1.0.0"] = lifecycleOldID
	for _, key := range []string{"running", "stopped"} {
		f.workloads[key] = lifecycleWorkload{ID: strings.Repeat(map[string]string{"running": "1", "stopped": "2"}[key], 64), Service: key, Project: "example-stack", Reference: "registry.example.com/side:1.0.0", ImageID: lifecycleOldID, Running: key == "running"}
	}
	return f
}

func assertNoServiceAdmission(t *testing.T, f *lifecycleFixture) {
	t.Helper()
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
	require.Zero(t, f.tagWrites)
	require.Empty(t, lifecyclePoints(t, f))
	var count int64
	require.NoError(t, f.db.Table("runtime_receipts").Count(&count).Error)
	require.Zero(t, count)
}

func TestServiceScopePreflightNames(t *testing.T) {
	for _, test := range []struct {
		name     string
		services []string
		code     string
		point    string
	}{
		{"unknown", []string{"missing"}, "update.invalid-target", ""},
		{"mixed-unknown", []string{"app", "missing"}, "update.invalid-target", ""},
		{"explicit", []string{"app"}, "", ""},
		{"empty-whole-stack", nil, "", ""},
		// A restore covers the whole stack; a selection with a point is refused.
		{"restore-with-services", []string{"app"}, "update.invalid-target", "example-point"},
	} {
		t.Run(test.name, func(t *testing.T) {
			f := serviceScopeFixture(t, serviceScopeSource)
			current := f.establishBaseline()
			f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
				return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{
					"app": {UpdateInfo: &imageDTO.UpdateInfo{}}, "running": {UpdateInfo: &imageDTO.UpdateInfo{}}, "stopped": {UpdateInfo: &imageDTO.UpdateInfo{}},
				}}, nil
			})
			intent, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: test.services, PointID: test.point})
			if test.code != "" {
				lifecycleCode(t, err, test.code)
				require.Equal(t, 400, imageDTO.Neutral(err, "unexpected", 500).Status)
			} else {
				require.NoError(t, err)
				require.Equal(t, test.services, intent.Request.Services)
			}
			assertNoServiceAdmission(t, f)
		})
	}
}

func TestServiceScopeIgnoresUnselectedCheckFailures(t *testing.T) {
	f := serviceScopeFixture(t, serviceScopeSource)
	current := f.establishBaseline()
	f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
		return &projectDTO.UpdateInfo{ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {UpdateInfo: &imageDTO.UpdateInfo{}}}}, nil
	})
	_, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
	require.NoError(t, err, "unselected failed or absent observations cannot veto the selected check")
	assertNoServiceAdmission(t, f)
}

func TestServiceScopeCouplingBeforeAdmission(t *testing.T) {
	for _, direction := range []string{"selected-consumer", "unselected-consumer"} {
		for _, link := range []string{"network_mode: service:app", "ipc: service:app", "pid: service:app", "volumes_from: [app:ro]"} {
			t.Run(direction+"/"+strings.Split(link, ":")[0], func(t *testing.T) {
				source := strings.Replace(serviceScopeSource, "    network_mode: none\n    labels:", "    "+link+"\n    labels:", 1)
				f := serviceScopeFixture(t, source)
				current := f.establishBaseline()
				services := []string{"app"}
				if direction == "selected-consumer" {
					services = []string{"running"}
				}
				_, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: services})
				lifecycleCode(t, err, "update.service-coupled")
				assertNoServiceAdmission(t, f)
			})
		}
	}
	t.Run("stopped-dependent", func(t *testing.T) {
		source := strings.Replace(strings.Replace(serviceScopeSource, "    depends_on: [stopped]\n", "", 1), "  stopped:\n", "  stopped:\n    pid: service:app\n", 1)
		f := serviceScopeFixture(t, source)
		current := f.establishBaseline()
		_, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
		lifecycleCode(t, err, "update.service-coupled")
		assertNoServiceAdmission(t, f)
	})
}

func TestServiceScopeSharedImageBeforeCheck(t *testing.T) {
	for _, source := range []string{
		"services:\n  app:\n    image: &shared registry.example.com/app:1.0.0\n  running:\n    image: *shared\n  stopped:\n    image: registry.example.com/side:1.0.0\n",
		"x-image: &shared\n  image: registry.example.com/app:1.0.0\nservices:\n  app:\n    <<: *shared\n  running:\n    <<: *shared\n  stopped:\n    image: registry.example.com/side:1.0.0\n",
	} {
		t.Run(source[:8], func(t *testing.T) {
			f := serviceScopeFixture(t, source)
			current := f.establishBaseline()
			f.service.SetLifecycleChecker(func(context.Context, string) (*projectDTO.UpdateInfo, error) {
				t.Error("shared binding must be refused before check")
				return nil, errors.New("unexpected check")
			})
			_, err := f.service.PrepareImageLifecycle(t.Context(), f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: current.SavedRevision, Services: []string{"app"}})
			lifecycleCode(t, err, "update.image-binding-unsupported")
			assertNoServiceAdmission(t, f)
		})
	}
}

func TestServiceScopeExecutionAndWholeStackPoint(t *testing.T) {
	for _, outcome := range []string{"success", "failure"} {
		t.Run(outcome, func(t *testing.T) {
			f := serviceScopeFixture(t, serviceScopeSource)
			f.establishBaseline()
			before := map[string]lifecycleWorkload{"running": f.workloads["running"], "stopped": f.workloads["stopped"]}
			intent := f.intent("version")
			intent.Request.Services = []string{"app"}
			f.up = func(_ context.Context, model *compose.Project) error {
				require.Len(t, model.Services, 1)
				require.Contains(t, model.Services, "app")
				require.Empty(t, model.Services["app"].DependsOn)
				require.Equal(t, "never", model.Services["app"].PullPolicy)
				if outcome == "failure" {
					return errors.New("synthetic selected service create failure")
				}
				return nil
			}
			// A moved unselected tag must neither be retagged nor block this update.
			f.tags["registry.example.com/side:1.0.0"] = lifecycleNewID
			out := f.wait(f.admit(intent))
			want := activityDTO.StatusSuccess
			if outcome == "failure" {
				want = activityDTO.StatusFailed
			}
			require.Equal(t, want, out.Status, "%+v", out.ImageLifecycle)
			require.NotNil(t, out.ImageLifecycle)
			if outcome == "success" {
				require.True(t, out.ImageLifecycle.Verified)
			} else {
				require.Equal(t, "update.deploy-failed", out.ImageLifecycle.Code)
			}
			require.NotEmpty(t, out.ImageLifecycle.RestorePointID)
			_, saved, err := f.service.lifecycleStore().Point(t.Context(), out.ImageLifecycle.RestorePointID)
			require.NoError(t, err)
			require.Len(t, saved.Services, 3)
			require.True(t, saved.Services["running"].Running)
			require.False(t, saved.Services["stopped"].Running)
			f.mu.Lock()
			defer f.mu.Unlock()
			require.Equal(t, before["running"], f.workloads["running"])
			require.Equal(t, before["stopped"], f.workloads["stopped"])
			require.Empty(t, f.stopSignals)
			require.Equal(t, lifecycleNewID, f.tags["registry.example.com/side:1.0.0"], "unselected tag must not be selected or compensated")
		})
	}
}

func TestServiceScopeStackExclusion(t *testing.T) {
	for _, activeScope := range []string{"service", "whole", "window"} {
		t.Run(activeScope, func(t *testing.T) {
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
			var enteredOnce sync.Once
			f.up = func(context.Context, *compose.Project) error {
				enteredOnce.Do(func() { close(entered) })
				<-release
				return nil
			}
			intent := f.intent("version")
			if activeScope == "service" {
				intent.Request.Services = []string{"app"}
			}
			if activeScope == "window" {
				serviceScopePolicy(t, f, intent)
			}
			candidates := map[string]*dto.LifecycleIntent{}
			for _, contender := range []string{"service", "whole", "window"} {
				candidate := f.intent("version")
				if contender == "service" {
					candidate.Request.Services = []string{"app"}
				}
				if contender == "window" {
					serviceScopePolicy(t, f, candidate)
				}
				candidates[contender] = candidate
			}
			active := f.admit(intent)
			<-entered
			for _, contender := range []string{"service", "whole", "window"} {
				if contender == activeScope {
					continue
				}
				candidate := candidates[contender]
				// A window child uses the identical runtime operation and stack target;
				// policy authority is checked by its own admission path before this gate.
				_, err := f.runtime.Admit(t.Context(), activity.RuntimeCommand{Store: f.binding, Key: uuid.NewString(), Actor: user.Actor{ID: "operator", Username: "operator"}, Operation: activity.RuntimeUpdateProject, Target: activity.RuntimeTarget{Kind: "stack", ID: f.project.ID, Name: new("example-stack")}, Options: activity.RuntimeOptions{Lifecycle: candidate}})
				var busy *activity.GateConflict
				require.ErrorAs(t, err, &busy, contender)
				require.Equal(t, active.ActivityID, busy.RunID, contender)
			}
			close(release)
			require.Equal(t, activityDTO.StatusSuccess, f.wait(active).Status)
		})
	}
}

func TestServiceScopeExecutorRejectsOutOfScopeSelection(t *testing.T) {
	f := serviceScopeFixture(t, serviceScopeSource)
	f.establishBaseline()
	intent := f.intent("version")
	intent.Request.Services = []string{"running"}
	out := f.wait(f.admit(intent))
	require.Equal(t, activityDTO.StatusFailed, out.Status)
	require.Equal(t, "update.invalid-target", out.ImageLifecycle.Code)
	require.Empty(t, lifecyclePoints(t, f))
	var points []life.Point
	require.NoError(t, f.db.Find(&points).Error)
	require.Empty(t, points)
	require.Zero(t, f.pulls)
	require.Zero(t, f.deploys)
}

func serviceScopePolicy(t *testing.T, f *lifecycleFixture, intent *dto.LifecycleIntent) {
	t.Helper()
	store := policy.Store{DB: f.db.DB}
	p, err := store.Policy(t.Context(), f.binding.EnvironmentID, f.registration, f.project.ID)
	require.NoError(t, err)
	if p.Revision == 0 {
		p.Mode = "scheduled"
		p.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
		p.ActorID = "operator"
		p.TargetIdentity = intent.TargetIdentity
		require.NoError(t, f.db.Exec("INSERT OR IGNORE INTO environments(id,name,api_url,enabled) VALUES(?,?,?,1)", f.binding.EnvironmentID, "Example", "https://agent.example.com").Error)
		require.NoError(t, store.SavePolicy(t.Context(), p, 0))
		p.Revision = 1
	}
	child, fresh, err := store.PrepareChild(t.Context(), p, uuid.NewString())
	require.NoError(t, err)
	require.True(t, fresh)
	intent.Policy = &dto.PolicyBinding{PolicyID: p.ID, Revision: p.Revision, JobRunID: child.JobRunID, ChildID: child.ID}
}
