// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Real project service and Compose SDK HTTP boundary proofs.

package activity_test

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	dockertypes "go.getarcane.app/docker/types"
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
	require.NotEmpty(t, run.Output[len(run.Output)-1].Text, "the final accepted SDK progress line must drain")
	var codes []string
	require.NoError(t, f.db.Raw("SELECT code FROM activity_messages WHERE activity_id = ? ORDER BY rowid", runID).Scan(&codes).Error)
	require.Greater(t, len(codes), 2, "SDK output should precede the final message")
	require.Equal(t, "run.started", codes[0])
	require.Equal(t, "activity.success", codes[len(codes)-1])
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
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	f.http.mu.Lock()
	f.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
		if r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			select {
			case <-entered:
			default:
				close(entered)
			}
			select {
			case <-release:
			case <-r.Context().Done():
			}
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
	unblock()
	snapshot, err := f.waitLocal(admitted.ActivityID)
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
	snapshot, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status))
	for _, event := range f.http.snapshot() {
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create"), f.debugEvents())
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/start"), f.debugEvents())
	}
}

func TestRuntimeExecutorBuildCollaboratorErrorBeforeUp(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    pull_policy: build\n    build:\n      context: .\n      dockerfile: Missing.Dockerfile\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	waitCtx, cancel := context.WithTimeout(t.Context(), 8*time.Second)
	defer cancel()
	snapshot, err := f.runtime.WaitLocal(waitCtx, activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status), f.debugEvents())
	for _, event := range f.http.snapshot() {
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create"), f.debugEvents())
		require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/start"), f.debugEvents())
	}
	var buildFailures int64
	require.NoError(t, f.db.Table("events").Where("resource_type = ? AND type = ?", "image", "image.error").Count(&buildFailures).Error)
	require.Positive(t, buildFailures, "actual BuildService should record a failed build")
}

func TestRuntimeExecutorComposeSDKCancellationMatrix(t *testing.T) {
	for _, cause := range []string{"user", "application", "deadline"} {
		t.Run(cause, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
			var app context.Context
			var stop context.CancelFunc
			if cause == "deadline" {
				app, stop = context.WithTimeout(context.Background(), 4*time.Second)
			} else {
				app, stop = context.WithCancel(context.Background())
			}
			defer stop()
			f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: app, Store: f.store, Executor: f.adapter})
			entered, cancelled, release := make(chan struct{}), make(chan struct{}), make(chan struct{})
			var releaseOnce sync.Once
			unblock := func() { releaseOnce.Do(func() { close(release) }) }
			defer unblock()
			f.http.mu.Lock()
			f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
				if r.Method != http.MethodPost || !strings.HasSuffix(r.URL.Path, "/containers/"+runtimeContainerID+"/start") {
					return false
				}
				close(entered)
				select {
				case <-r.Context().Done():
					close(cancelled)
				case <-release:
				}
				http.Error(w, "synthetic cancelled request", http.StatusServiceUnavailable)
				return true
			}
			f.http.mu.Unlock()
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			admitted, err := f.runtime.Admit(t.Context(), command)
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(3 * time.Second):
				t.Fatal("Compose did not reach the SDK start call")
			}
			if cause == "user" {
				cancelCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
				_, err = f.runtime.Cancel(cancelCtx, activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID}, usertypes.Actor{ID: "example-owner", Username: "Operator"})
				cancel()
				require.NoError(t, err)
			} else if cause == "application" {
				stop()
			}
			select {
			case <-cancelled:
			case <-time.After(5 * time.Second):
				t.Error("actual Compose SDK HTTP request did not receive lifecycle cancellation")
			}
			unblock()
			waitCtx, cancel := context.WithTimeout(t.Context(), 5*time.Second)
			defer cancel()
			snapshot, err := f.runtime.WaitLocal(waitCtx, activity.RuntimeRunReference{Store: f.store, RunID: admitted.ActivityID})
			require.NoError(t, err)
			if cause == "user" {
				require.Equal(t, "cancelled", string(snapshot.Status))
			} else {
				require.Equal(t, "failed", string(snapshot.Status))
			}
		})
	}
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
	snapshot, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status), f.debugEvents())
	var finalCount int64
	require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = ?", admitted.ActivityID, "runtime.failed").Count(&finalCount).Error)
	require.EqualValues(t, 1, finalCount)
}

// This test-only port delegates the actual ProjectService, then holds a local
// progress producer after the SDK returns. Server handler lifetime is not the join.
type runtimeHeldProjectPort struct {
	*project.ProjectService
	entered, release, wrote chan struct{}
}

func (p *runtimeHeldProjectPort) DeployProject(ctx context.Context, id string, actor usertypes.Actor, options *projecttypes.DeployOptions) error {
	if err := p.ProjectService.DeployProject(ctx, id, actor, options); err != nil {
		return err
	}
	progress, ok := ctx.Value(dockertypes.ProgressWriterKey{}).(io.Writer)
	if !ok {
		return errors.New("missing real service progress writer")
	}
	close(p.entered)
	<-p.release
	_, err := io.WriteString(progress, "local producer tail without newline")
	close(p.wrote)
	return err
}

func TestRuntimeExecutorRealComposeOutputDrainsBeforeTerminal(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
	entered, release, wrote := make(chan struct{}), make(chan struct{}), make(chan struct{})
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	defer unblock()
	held := &runtimeHeldProjectPort{ProjectService: f.projects, entered: entered, release: release, wrote: wrote}
	f.adapter = activity.NewRuntimeServiceAdapter(held, f.containers)
	f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: f.store, Executor: f.adapter})
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	select {
	case <-entered:
	case <-time.After(3 * time.Second):
		t.Fatal("actual ProjectService and held local producer did not return")
	}
	var unsettled, finalCount int64
	require.NoError(t, f.db.Table("runtime_receipts").Where("run_id = ? AND settled_at IS NULL", admitted.ActivityID).Count(&unsettled).Error)
	require.EqualValues(t, 1, unsettled, "local producer holds the durable scope")
	require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = ?", admitted.ActivityID, "activity.success").Count(&finalCount).Error)
	require.Zero(t, finalCount)
	competing := f.command(activity.RuntimeDownProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	refused, err := f.runtime.Admit(t.Context(), competing)
	require.Empty(t, refused.ActivityID)
	var conflict *activity.GateConflict
	require.ErrorAs(t, err, &conflict)
	lockCtx, cancelLock := context.WithTimeout(t.Context(), 5*time.Second)
	defer cancelLock()
	writePool, err := sqlitestore.Open(lockCtx, f.dbPath, true, false)
	require.NoError(t, err)
	defer writePool.Close()
	lock, err := writePool.BeginTx(lockCtx, nil)
	require.NoError(t, err)
	defer lock.Rollback()
	_, err = lock.ExecContext(lockCtx, "UPDATE projects SET status = status WHERE id = ?", "example-project")
	require.NoError(t, err)
	readPool, err := sqlitestore.Open(lockCtx, f.dbPath, true, true)
	require.NoError(t, err)
	defer readPool.Close()
	unblock()
	select {
	case <-wrote:
	case <-time.After(2 * time.Second):
		t.Fatal("local output producer did not return")
	}
	sqlDB, err := f.db.DB.DB()
	require.NoError(t, err)
	require.Eventually(t, func() bool { return sqlDB.Stats().InUse == 1 }, 2*time.Second, 10*time.Millisecond,
		"the real ActivityService append must be waiting on the SQLite write lock")
	var heldReceipt, heldFinal int
	require.NoError(t, readPool.QueryRowContext(lockCtx, "SELECT count(*) FROM runtime_receipts WHERE run_id = ? AND settled_at IS NULL", admitted.ActivityID).Scan(&heldReceipt))
	require.Equal(t, 1, heldReceipt)
	require.NoError(t, readPool.QueryRowContext(lockCtx, "SELECT count(*) FROM activity_messages WHERE activity_id = ? AND code = 'activity.success'", admitted.ActivityID).Scan(&heldFinal))
	require.Zero(t, heldFinal)
	require.NoError(t, lock.Rollback())
	snapshot, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, "success", string(snapshot.Status), f.debugEvents())
	var run activity.Activity
	require.NoError(t, f.db.First(&run, "id = ?", admitted.ActivityID).Error)
	require.NotEmpty(t, run.Output)
	require.True(t, strings.HasSuffix(run.Output[len(run.Output)-1].Text, "local producer tail without newline"))
	var codes []string
	require.NoError(t, f.db.Raw("SELECT code FROM activity_messages WHERE activity_id = ? ORDER BY rowid", admitted.ActivityID).Scan(&codes).Error)
	require.Greater(t, len(codes), 2)
	require.Equal(t, "activity.success", codes[len(codes)-1])
}

func TestRuntimeExecutorProjectSlotsAtLimitOne(t *testing.T) {
	for _, operation := range []activity.RuntimeOperation{activity.RuntimeDeployProject, activity.RuntimeRedeployProject} {
		t.Run(string(operation), func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			app, stop := context.WithTimeout(t.Context(), 8*time.Second)
			defer stop()
			f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: app, Store: f.store, Executor: f.adapter})
			require.NoError(t, f.settings.SetIntSetting(t.Context(), "maxConcurrentActivities", 1))
			f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n")
			f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-hash"})
			legacy, err := f.activity.StartActivity(t.Context(), activity.StartActivityRequest{Type: activitytypes.TypeImagePull, Queue: true})
			require.NoError(t, err)
			entered, release := make(chan struct{}), make(chan struct{})
			var once sync.Once
			unblock := func() { once.Do(func() { close(release) }) }
			defer unblock()
			var observed sync.Once
			f.http.mu.Lock()
			f.http.onCall = func(_ http.ResponseWriter, r *http.Request) bool {
				// Deploy reaches image preparation; redeploy first reaches its actual
				// preliminary ImageService pull. Both execute under the parent's one slot.
				inspect := r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json")
				pull := r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create")
				if (operation == activity.RuntimeDeployProject && inspect) || (operation == activity.RuntimeRedeployProject && pull) {
					observed.Do(func() { close(entered) })
					select {
					case <-release:
					case <-r.Context().Done():
					}
				}
				return false
			}
			f.http.mu.Unlock()
			command := f.command(operation, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", ForceRecreate: true, RemoveOrphans: true, RecreateVolumes: true}
			admitted, err := f.runtime.Admit(t.Context(), command)
			require.NoError(t, err)
			// Any assertion failure must unblock handlers and join the accepted owner
			// before fixture teardown closes the SQLite store and synthetic HTTP server.
			defer func() {
				unblock()
				f.activity.CompleteActivity(context.WithoutCancel(t.Context()), legacy.ID, activitytypes.StatusSuccess, "Complete", nil)
				_, _ = f.waitLocal(admitted.ActivityID)
			}()
			require.Equal(t, activitytypes.StatusQueued, admitted.Activity.Status)
			require.Equal(t, command.Actor.ID, admitted.Activity.StartedBy.UserID)
			var receipt activity.RuntimeReceipt
			require.Eventually(t, func() bool {
				return f.db.First(&receipt, "run_id = ?", admitted.ActivityID).Error == nil && *receipt.ExecutorState != "pending"
			}, 3*time.Second, 5*time.Millisecond)
			require.Equal(t, "accepted", *receipt.ExecutorState)
			require.Nil(t, receipt.ExecutorStartedAt)
			require.Empty(t, runtimeEffectEvents(f.http.snapshot()), "no create/start/pull while the parent is queued")
			detail, err := f.activity.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusQueued, detail.Activity.Status)
			joined, err := f.runtime.Admit(t.Context(), command)
			require.NoError(t, err)
			require.Equal(t, admitted.ActivityID, joined.ActivityID)
			_, err = f.activity.CompleteActivity(t.Context(), legacy.ID, activitytypes.StatusSuccess, "Complete", nil)
			require.NoError(t, err)
			select {
			case <-entered:
			case <-time.After(8 * time.Second):
				t.Fatal("actual image preparation or preliminary pull did not enter at limit one")
			}
			require.NoError(t, f.db.First(&receipt, "run_id = ?", admitted.ActivityID).Error)
			require.Equal(t, "executing", *receipt.ExecutorState)
			detail, err = f.activity.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusRunning, detail.Activity.Status)
			for _, e := range f.http.snapshot() {
				require.False(t, e.Method == http.MethodPost && strings.HasSuffix(e.Path, "/containers/create"), "preparation must join before Up")
			}
			unblock()
			snapshot, err := f.waitLocal(admitted.ActivityID)
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusSuccess, snapshot.Status, f.debugEvents())
			var creates, starts, pulls int
			for _, e := range f.http.snapshot() {
				if e.Method != http.MethodPost {
					continue
				}
				if strings.HasSuffix(e.Path, "/containers/create") {
					creates++
				}
				if strings.HasSuffix(e.Path, "/start") {
					starts++
				}
				if strings.HasSuffix(e.Path, "/images/create") {
					pulls++
				}
			}
			require.Equal(t, 1, creates)
			require.Equal(t, 1, starts)
			if operation == activity.RuntimeRedeployProject {
				require.Equal(t, 1, pulls)
			} else {
				require.Zero(t, pulls)
			}
			var lifecycle, receipts, finals int64
			require.NoError(t, f.db.Model(&activity.Activity{}).Where("id = ?", admitted.ActivityID).Count(&lifecycle).Error)
			require.NoError(t, f.db.Model(&activity.RuntimeReceipt{}).Where("run_id = ?", admitted.ActivityID).Count(&receipts).Error)
			require.NoError(t, f.db.Table("activity_messages").Where("activity_id = ? AND code = 'activity.success'", admitted.ActivityID).Count(&finals).Error)
			require.EqualValues(t, 1, lifecycle)
			require.EqualValues(t, 1, receipts)
			require.EqualValues(t, 1, finals)
			var row activity.Activity
			require.NoError(t, f.db.First(&row, "id = ?", admitted.ActivityID).Error)
			require.NotEmpty(t, row.Output)
			next, err := f.activity.StartActivity(t.Context(), activity.StartActivityRequest{Type: activitytypes.TypeImagePull, Queue: true})
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusRunning, next.Status)
			_, err = f.activity.CompleteActivity(t.Context(), next.ID, activitytypes.StatusSuccess, "Complete", nil)
			require.NoError(t, err)
		})
	}
}

func TestRuntimeExecutorBuildErrorReturnsParentSlotAtLimitOne(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	app, stop := context.WithTimeout(t.Context(), 8*time.Second)
	defer stop()
	f.runtime = activity.NewRuntimeService(f.activity, f.adapter, activity.RuntimeLocalConfig{ApplicationContext: app, Store: f.store, Executor: f.adapter})
	require.NoError(t, f.settings.SetIntSetting(t.Context(), "maxConcurrentActivities", 1))
	f.project("example-project", "example-project", "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n    pull_policy: build\n    build:\n      context: .\n      dockerfile: Missing.Dockerfile\n")
	command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	admitted, err := f.runtime.Admit(t.Context(), command)
	require.NoError(t, err)
	snapshot, err := f.waitLocal(admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusFailed, snapshot.Status)
	var failures int64
	require.NoError(t, f.db.Table("events").Where("resource_type = 'image' AND type = 'image.error' AND user_id = ?", command.Actor.ID).Count(&failures).Error)
	require.Positive(t, failures, "actual BuildService returns from missing Dockerfile preparation; this is not successful BuildKit transport")
	for _, e := range f.http.snapshot() {
		require.False(t, e.Method == http.MethodPost && (strings.HasSuffix(e.Path, "/containers/create") || strings.HasSuffix(e.Path, "/start")))
	}
	next, err := f.activity.StartActivity(t.Context(), activity.StartActivityRequest{Type: activitytypes.TypeImagePull, Queue: true})
	require.NoError(t, err)
	require.Equal(t, activitytypes.StatusRunning, next.Status)
	_, err = f.activity.CompleteActivity(t.Context(), next.ID, activitytypes.StatusSuccess, "Complete", nil)
	require.NoError(t, err)
}
