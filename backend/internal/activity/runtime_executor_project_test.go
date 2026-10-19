// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real project service and Compose SDK HTTP boundary proofs.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

func runtimeCreatedLabels(t *testing.T, events []runtimeHTTPEvent) map[string]string {
	t.Helper()
	for _, event := range events {
		if event.Method != http.MethodPost || !strings.HasSuffix(event.Path, "/containers/create") {
			continue
		}
		var body struct {
			Labels map[string]string `json:"Labels"`
		}
		require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
		require.NotEmpty(t, body.Labels["com.docker.compose.config-hash"])
		return body.Labels
	}
	t.Fatal("Compose did not create a container")
	return nil
}

func TestRuntimeExecutorProjectDownAndRestart(t *testing.T) {
	for _, operation := range []activity.RuntimeOperation{activity.RuntimeDownProject, activity.RuntimeRestartProject} {
		t.Run(string(operation), func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
			f.http.setComposeContainer(true)
			command := f.command(operation, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			if operation == activity.RuntimeRestartProject {
				command.Options.RestartServices = []string{"app"}
			}
			runID := f.run(command)
			require.NotEmpty(t, runID)
			events := f.http.snapshot()
			require.NotEmpty(t, events)
			var stop, remove, restart int
			for _, event := range events {
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/stop") {
					stop++
				}
				if event.Method == http.MethodDelete && strings.Contains(event.Path, "/containers/") {
					remove++
					require.NotContains(t, event.Query, "v=1", f.debugEvents())
				}
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/restart") {
					restart++
				}
			}
			if operation == activity.RuntimeDownProject {
				require.Equal(t, 1, stop)
				require.Equal(t, 1, remove)
				require.Zero(t, restart)
			} else {
				require.Zero(t, stop)
				require.Zero(t, remove)
				require.Equal(t, 1, restart)
			}
		})
	}
}

func TestRuntimeExecutorRestartSelectedAndWholeServiceSets(t *testing.T) {
	for _, tc := range []struct {
		name     string
		selected []string
		want     []string
	}{
		{name: "selected", selected: []string{"app"}, want: []string{runtimeContainerID}},
		{name: "whole", want: []string{runtimeContainerID, runtimeSecondContainerID}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n  worker:\n    image: registry.example.com/worker:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-app-hash"})
			f.http.setComposeContainer(true)
			f.http.setSecondComposeContainer(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "worker", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-worker-hash"})
			command := f.command(activity.RuntimeRestartProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.RestartServices = tc.selected
			f.run(command)
			var actual []string
			for _, event := range f.http.snapshot() {
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/restart") {
					parts := strings.Split(event.Path, "/")
					actual = append(actual, parts[len(parts)-2])
				}
			}
			require.ElementsMatch(t, tc.want, actual, f.debugEvents())
		})
	}
}

func TestRuntimeExecutorChangedProjectFileStopsBeforeCompose(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	path := f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	command := f.command(activity.RuntimeDownProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project", Name: new("example_stack")})
	_, err := f.adapter.ResolveRuntimeTarget(t.Context(), command.Target)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(filepath.Join(path, "compose.yaml"), []byte("name: changed_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n"), 0644))
	f.http.setLabels(map[string]string{"com.docker.compose.project": "changed_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	f.http.setComposeContainer(true)
	err = f.adapter.ExecuteRuntime(t.Context(), command, io.Discard)
	for _, event := range f.http.snapshot() {
		require.False(t, strings.Contains(event.Path, "/containers/") && event.Method == http.MethodPost)
	}
	require.ErrorContains(t, err, "runtime.target-changed")
}

func TestRuntimeExecutorProjectDeploy(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", ForceRecreate: true, RemoveOrphans: true, RecreateVolumes: true}
	runID := f.run(command)
	var run activity.Activity
	require.NoError(t, f.db.First(&run, "id = ?", runID).Error)
	require.NotEmpty(t, run.Output, "real Compose progress must use the runtime capture")
	var create, start int
	for _, event := range f.http.snapshot() {
		if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") {
			create++
		}
		if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/start") {
			start++
		}
	}
	require.Equal(t, 1, create, f.debugEvents())
	require.Equal(t, 1, start, f.debugEvents())
}

func TestRuntimeExecutorDeployForceRecreateAtSDK(t *testing.T) {
	for _, force := range []bool{false, true} {
		t.Run(map[bool]string{false: "diverged", true: "force"}[force], func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-initial-hash"})
			initial := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			initial.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			f.run(initial)
			labels := runtimeCreatedLabels(t, f.http.snapshot())
			f.http.setLabels(labels)
			before := len(f.http.snapshot())
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", ForceRecreate: force}
			f.run(command)
			var removes, creates int
			for _, event := range f.http.snapshot()[before:] {
				if event.Method == http.MethodDelete && strings.Contains(event.Path, "/containers/") {
					removes++
				}
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") {
					creates++
				}
			}
			if force {
				require.Equal(t, 1, removes, f.debugEvents())
				require.Equal(t, 1, creates, f.debugEvents())
			} else {
				require.Zero(t, removes, f.debugEvents())
				require.Zero(t, creates, f.debugEvents())
			}
		})
	}
}

func TestRuntimeExecutorDeployPullPolicyAtSDK(t *testing.T) {
	for _, tc := range []struct {
		policy string
		pulls  int
	}{{"never", 0}, {"always", 1}} {
		t.Run(tc.policy, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: tc.policy}
			f.run(command)
			var pulls, creates int
			for _, event := range f.http.snapshot() {
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/images/create") {
					pulls++
				}
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") {
					creates++
				}
			}
			require.Equal(t, tc.pulls, pulls, f.debugEvents())
			require.Equal(t, 1, creates, f.debugEvents())
		})
	}
}

func TestRuntimeExecutorDeployRemoveOrphansAtSDK(t *testing.T) {
	for _, remove := range []bool{false, true} {
		t.Run(map[bool]string{false: "keep", true: "remove"}[remove], func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-initial-hash"})
			initial := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			initial.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			f.run(initial)
			f.http.setLabels(runtimeCreatedLabels(t, f.http.snapshot()))
			f.http.setSecondComposeContainer(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "old", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-old-hash"})
			before := len(f.http.snapshot())
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", RemoveOrphans: remove}
			f.run(command)
			var deletes int
			for _, event := range f.http.snapshot()[before:] {
				if event.Method == http.MethodDelete && strings.Contains(event.Path, runtimeSecondContainerID) {
					deletes++
				}
			}
			if remove {
				require.Equal(t, 1, deletes, f.debugEvents())
			} else {
				require.Zero(t, deletes, f.debugEvents())
			}
		})
	}
}

func TestRuntimeExecutorDeployRecreateVolumesAtSDK(t *testing.T) {
	for _, recreate := range []bool{false, true} {
		t.Run(map[bool]string{false: "preserve", true: "recreate"}[recreate], func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    volumes:\n      - data:/data\nvolumes:\n  data: {}\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-initial-hash"})
			initial := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			initial.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			f.run(initial)
			f.http.setLabels(runtimeCreatedLabels(t, f.http.snapshot()))
			f.http.setVolumeHash("example-stale-volume-hash")
			before := len(f.http.snapshot())
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", RecreateVolumes: recreate}
			f.run(command)
			var deletes, creates int
			for _, event := range f.http.snapshot()[before:] {
				if event.Method == http.MethodDelete && strings.Contains(event.Path, "/volumes/") {
					deletes++
				}
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/volumes/create") {
					creates++
				}
			}
			if recreate {
				require.Equal(t, 1, deletes, f.debugEvents())
				require.Equal(t, 1, creates, f.debugEvents())
			} else {
				require.Zero(t, deletes, f.debugEvents())
				require.Zero(t, creates, f.debugEvents())
			}
		})
	}
}

func TestRuntimeExecutorProjectRedeployPreliminaryPull(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create") {
			http.Error(w, "synthetic pull failure", http.StatusInternalServerError)
			return true
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeRedeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	f.run(command)
	firstPull, firstCreate := -1, -1
	for i, event := range f.http.snapshot() {
		if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/images/create") && firstPull < 0 {
			firstPull = i
		}
		if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") && firstCreate < 0 {
			firstCreate = i
		}
	}
	require.NotEqual(t, -1, firstPull, f.debugEvents())
	require.NotEqual(t, -1, firstCreate, f.debugEvents())
	require.Less(t, firstPull, firstCreate)
}

func TestRuntimeExecutorProjectPreparationBeforeUp(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	entered := make(chan struct{})
	release := make(chan struct{})
	f.http.mu.Lock()
	f.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			select {
			case <-entered:
			default:
				close(entered)
			}
			<-release
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("image inspection was not reached")
	}
	for _, event := range f.http.snapshot() {
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create"), f.debugEvents())
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/start"), f.debugEvents())
	}
	close(release)
	snapshot, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, err)
	require.Equal(t, "success", string(snapshot.Status), f.debugEvents())
}

func TestRuntimeExecutorProjectPreparationErrorHasNoUp(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			http.Error(w, "synthetic image inspection failure", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	snapshot, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status))
	for _, event := range f.http.snapshot() {
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create"), f.debugEvents())
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/start"), f.debugEvents())
	}
}

func TestRuntimeExecutorComposeCallKeepsLifecycleCancellation(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	app, stop := context.WithCancel(context.Background())
	defer stop()
	f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: app, Store: f.store, Executor: f.adapter})
	entered := make(chan struct{})
	cancelled := make(chan struct{})
	release := make(chan struct{})
	f.http.mu.Lock()
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/start") {
			close(entered)
			select {
			case <-r.Context().Done():
				close(cancelled)
			case <-release:
			}
			http.Error(w, "synthetic cancelled request", http.StatusServiceUnavailable)
			return true
		}
		return false
	}
	f.http.mu.Unlock()
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(10 * time.Second):
		t.Fatal("Compose did not reach the SDK start call")
	}
	stop()
	observed := false
	select {
	case <-cancelled:
		observed = true
	case <-time.After(2 * time.Second):
	}
	close(release)
	require.True(t, observed, "Compose SDK request lost the worker's cancellation")
	snapshot, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status))
}

func TestRuntimeExecutorRealComposeOutputFailureSettlesFailed(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	require.NoError(t, f.db.Exec(`CREATE TRIGGER example_reject_output BEFORE UPDATE OF output ON activities
		WHEN NEW.output != OLD.output BEGIN SELECT RAISE(ABORT, 'synthetic output refusal'); END`).Error)
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	snapshot, err := f.runtime.WaitLocal(t.Context(), activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status), f.debugEvents())
	var finalCount int64
	require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = ?", admitted.ActivityID, "runtime.failed").Count(&finalCount).Error)
	require.EqualValues(t, 1, finalCount)
}
