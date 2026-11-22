// Source: docklet-next/issues/568
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original real-Compose coordinator proof for no-pull baseline recording on externally started stacks.; M36 #568: record baseline without pulling images.

package activity_test

import (
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

const recordBaselineCompose = "name: example_stack\nservices:\n  app:\n    image: registry.example.com/app:1\n"
const recordBaselineImage = "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"

// Compose itself supplies the hash/labels. The target fixture has a separate, fresh
// database and an already running container, with no prior Docklet deploy.
func externalBaselineFixture(t *testing.T) (*runtimeServicesFixture, string) {
	seed := newRuntimeServicesFixture(t)
	seed.project("example-project", "example-project", recordBaselineCompose)
	seed.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "external-initial-hash"})
	initial := seed.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	initial.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
	seed.run(initial)
	labels := runtimeCreatedLabels(t, seed.http.snapshot())
	for key := range labels {
		if strings.HasPrefix(key, "io.docklet.") {
			delete(labels, key)
		}
	}
	f := newRuntimeServicesFixture(t)
	dir := f.project("example-project", "example-project", recordBaselineCompose)
	labels["com.docker.compose.project.working_dir"] = dir
	labels["com.docker.compose.project.config_files"] = filepath.Join(dir, "compose.yaml")
	f.http.setLabels(labels)
	f.http.setComposeContainer(true)
	require.NoError(t, f.settings.SetStringSetting(t.Context(), "defaultDeployPullPolicy", "always"))
	f.http.onCall = func(w http.ResponseWriter, r *http.Request) bool {
		path := r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(path, "/images/") && strings.HasSuffix(path, "/json"):
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": recordBaselineImage, "Os": "linux", "Architecture": "amd64", "RepoTags": []string{"registry.example.com/app:1"}, "Config": map[string]any{}})
			return true
		case r.Method == http.MethodPost && strings.HasSuffix(path, "/containers/create"):
			var config struct{ Labels map[string]string }
			require.NoError(t, json.NewDecoder(r.Body).Decode(&config))
			f.http.setLabels(config.Labels)
			return false
		case strings.HasSuffix(path, "/containers/json"):
			f.http.mu.Lock()
			id, running, current := f.http.activeID, f.http.running, f.http.labels
			f.http.mu.Unlock()
			if strings.Contains(r.URL.Query().Get("filters"), "com.docker.compose.hook") {
				_ = json.NewEncoder(w).Encode([]any{})
				return true
			}
			state := "exited"
			if running {
				state = "running"
			}
			_ = json.NewEncoder(w).Encode([]any{map[string]any{"Id": id, "Names": []string{"/example_stack-app-1"}, "Image": "registry.example.com/app:1", "ImageID": recordBaselineImage, "Labels": current, "State": state}})
			return true
		case strings.Contains(path, "/containers/") && strings.HasSuffix(path, "/json"):
			f.http.mu.Lock()
			id, running, current := f.http.activeID, f.http.running, f.http.labels
			f.http.mu.Unlock()
			state := "exited"
			if running {
				state = "running"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"Id": id, "Name": "/example_stack-app-1", "Image": recordBaselineImage, "Config": map[string]any{"Image": "registry.example.com/app:1", "Labels": current}, "State": map[string]any{"Status": state, "Running": running}, "NetworkSettings": map[string]any{"Networks": map[string]any{}}})
			return true
		}
		return false
	}
	return f, dir
}

func TestRecordBaselineRealCompose(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	for _, savedEdit := range []bool{false, true} {
		t.Run(map[bool]string{false: "unchanged-external", true: "saved-edit"}[savedEdit], func(t *testing.T) {
			f, dir := externalBaselineFixture(t)
			if savedEdit {
				require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(recordBaselineCompose+"    command: [echo, saved-change]\n"), 0644))
			}
			status, err := f.projects.ImageBaselineStatus(t.Context(), "example-project", f.store.RegistrationID)
			require.NoError(t, err)
			require.Equal(t, "update.baseline-unavailable", status.Code)
			require.Empty(t, status.SourceRunID)
			before := len(f.http.snapshot())
			f.http.mu.Lock()
			originalID := f.http.activeID
			f.http.mu.Unlock()
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never", ForceRecreate: false}
			runID := f.run(command)
			pulls, creates := 0, 0
			applied := false
			for _, event := range f.http.snapshot()[before:] {
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/images/create") {
					pulls++
				}
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") {
					creates++
					var input struct{ Cmd []string }
					require.NoError(t, json.Unmarshal([]byte(event.Body), &input))
					applied = strings.Join(input.Cmd, " ") == "echo saved-change"
				}
			}
			require.Zero(t, pulls, "recording a baseline must never pull, even with a default policy of always")
			if savedEdit {
				require.Equal(t, 1, creates)
				require.True(t, applied, "saved undeployed changes must be applied by the ordinary deploy")
			} else {
				require.Zero(t, creates, "unchanged external containers must not be recreated")
				f.http.mu.Lock()
				actualID := f.http.activeID
				f.http.mu.Unlock()
				require.Equal(t, originalID, actualID, "container identity must be unchanged")
			}
			status, err = f.projects.ImageBaselineStatus(t.Context(), "example-project", f.store.RegistrationID)
			require.NoError(t, err)
			require.True(t, status.Available, "the successful normal deploy must capture the baseline: %s", status.Code)
			require.Equal(t, runID, status.SourceRunID)
		})
	}
}

// An explicit request policy "never" wins over every service pull_policy (review M36-S1B-R01).
func TestRecordBaselineIgnoresServicePullPolicies(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	for _, policy := range []string{"always", "daily", "every_2h", "missing", "build"} {
		t.Run(policy, func(t *testing.T) {
			f, dir := externalBaselineFixture(t)
			extra := "    pull_policy: " + policy + "\n"
			if policy == "build" {
				extra += "    build: .\n"
			}
			require.NoError(t, os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte(recordBaselineCompose+extra), 0644))
			before := len(f.http.snapshot())
			command := f.command(activity.RuntimeDeployProject, activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			runID := f.run(command)
			for _, event := range f.http.snapshot()[before:] {
				require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/images/create"), "service pull_policy %s must not pull: %s", policy, event.Path)
				require.False(t, event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/build"), "no build: %s", event.Path)
			}
			status, err := f.projects.ImageBaselineStatus(t.Context(), "example-project", f.store.RegistrationID)
			require.NoError(t, err)
			require.True(t, status.Available, status.Code)
			require.Equal(t, runID, status.SourceRunID)
		})
	}
}
