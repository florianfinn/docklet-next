// Source: docklet-next/docs/design/runtime-control.md
// Revision: 2b938d4fceea508777a188307ba1f8bf356d78fc59ad898bf0adead266953182
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Fresh project identity and environment proofs at the actual Compose SDK HTTP boundary.

package activity_test

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	projecttypes "github.com/florianfinn/docklet-next/types/project"
	"github.com/stretchr/testify/require"
)

const identitySDKServices = "services:\n  app:\n    image: registry.example.com/app:1\n"

// Count every mutating HTTP verb, including DELETE, image pulls and build,
// network and volume requests. Read-only inspection is permitted by preflight.
func identitySDKEffects(events []runtimeHTTPEvent) []runtimeHTTPEvent {
	var effects []runtimeHTTPEvent
	for _, event := range events {
		switch event.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
		default:
			effects = append(effects, event)
		}
	}
	return effects
}

func identitySDKRewrite(t *testing.T, path, content string) {
	t.Helper()
	info, err := os.Stat(path)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, []byte(content), 0600))
	require.NoError(t, os.Chtimes(path, info.ModTime(), info.ModTime()))
}

func identitySDKExecute(t *testing.T, f *runtimeServicesFixture, command activity.RuntimeCommand) error {
	t.Helper()
	ctx, cancel := context.WithTimeout(t.Context(), 20*time.Second)
	defer cancel()
	return f.adapter.ExecuteRuntime(ctx, command, io.Discard)
}

func TestRuntimeExecutorIdentityFreshNameHasZeroEffects(t *testing.T) {
	for _, source := range []string{"primary", "auxiliary", "primary-symlink", "auxiliary-symlink", "base", "override", "selection", "file-list", "deploy-primary", "restart-primary", "redeploy-primary"} {
		t.Run(source, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			path := f.project("example-project", "example-project", "name: ${STACK_NAME}\n"+identitySDKServices)
			env := filepath.Join(path, ".env")
			require.NoError(t, os.WriteFile(env, []byte("STACK_NAME=first_stack\n"), 0600))
			edit, content := env, "STACK_NAME=second_stack\n"
			switch source {
			case "auxiliary", "auxiliary-symlink":
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_ENV_FILES=aux.env\n"), 0600))
				edit = filepath.Join(path, "aux.env")
				require.NoError(t, os.WriteFile(edit, []byte("STACK_NAME=first_stack\n"), 0600))
			case "base":
				edit = filepath.Join(path, "compose.yaml")
				content = "name: second_stack\n" + identitySDKServices
			case "override":
				edit = filepath.Join(path, "compose.override.yaml")
				require.NoError(t, os.WriteFile(edit, []byte("name: first_stack\n"), 0600))
				content = "name: second_stack\n"
			case "selection":
				require.NoError(t, os.WriteFile(filepath.Join(path, "next.yaml"), []byte("name: second_stack\n"+identitySDKServices), 0600))
				content = "STACK_NAME=first_stack\nCOMPOSE_FILE=next.yaml\n"
			case "file-list":
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_FILE=compose.yaml:extra.yaml\nSTACK_NAME=first_stack\n"), 0600))
				edit = filepath.Join(path, "extra.yaml")
				require.NoError(t, os.WriteFile(edit, []byte("name: first_stack\n"), 0600))
				content = "name: second_stack\n"
			}
			if strings.HasSuffix(source, "symlink") {
				bytes, err := os.ReadFile(edit)
				require.NoError(t, err)
				require.NoError(t, os.Remove(edit))
				target := filepath.Join(t.TempDir(), "external.env")
				require.NoError(t, os.WriteFile(target, bytes, 0600))
				require.NoError(t, os.Symlink(target, edit))
				edit = target
			}
			// Inventory warms the ordinary parsed read cache; runtime must bypass it.
			_, err := f.projects.GetProjectDetails(t.Context(), "example-project", projecttypes.DetailsOptions{IncludeServiceConfigs: true})
			require.NoError(t, err)
			resolved, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			require.NoError(t, err)
			require.Equal(t, new("first_stack"), resolved.Target.Name)
			operation := activity.RuntimeDownProject
			switch source {
			case "deploy-primary":
				operation = activity.RuntimeDeployProject
			case "restart-primary":
				operation = activity.RuntimeRestartProject
			case "redeploy-primary":
				operation = activity.RuntimeRedeployProject
			}
			command := f.command(operation, resolved.Target)
			f.http.setLabels(map[string]string{"com.docker.compose.project": "second_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1"})
			f.http.setComposeContainer(true)
			identitySDKRewrite(t, edit, content)
			before := len(f.http.snapshot())
			err = identitySDKExecute(t, f, command)
			require.Empty(t, identitySDKEffects(f.http.snapshot()[before:]), "changed identity must issue zero mutating HTTP")
			t.Logf("source=%s mutatingHTTP=%d", source, len(identitySDKEffects(f.http.snapshot()[before:])))
			require.ErrorContains(t, err, "runtime.target-changed")
		})
	}
}

func TestRuntimeExecutorIdentityUncertainOwnershipHasZeroEffects(t *testing.T) {
	for _, change := range []string{"duplicate", "archived-duplicate", "same-mtime-peer", "missing-peer", "malformed-peer", "malformed-primary"} {
		for _, kind := range []string{"stack", "container"} {
			t.Run(change+"/"+kind, func(t *testing.T) {
				f := newRuntimeServicesFixture(t)
				path := f.project("example-project", "example-project", "name: example_stack\n"+identitySDKServices)
				peer := f.project("example-peer", "peer", "name: ${STACK_NAME}\n"+identitySDKServices)
				env := filepath.Join(peer, ".env")
				require.NoError(t, os.WriteFile(env, []byte("STACK_NAME=other_stack\n"), 0600))
				f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack", "com.docker.compose.service": "app", "com.docker.compose.container-number": "1"})
				f.http.setComposeContainer(true)
				target := activity.RuntimeTarget{Kind: "stack", ID: "example-project"}
				operation := activity.RuntimeDownProject
				if kind == "container" {
					target = activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID}
					operation = activity.RuntimeStopContainer
				}
				resolved, err := f.adapter.ResolveRuntimeTarget(t.Context(), target)
				require.NoError(t, err)
				command := f.command(operation, resolved.Target)
				switch change {
				case "duplicate", "archived-duplicate", "same-mtime-peer":
					identitySDKRewrite(t, env, "STACK_NAME=example_stack\n")
					if change == "archived-duplicate" {
						require.NoError(t, f.db.Model(&project.Project{}).Where("id = ?", "example-peer").Update("is_archived", true).Error)
					}
				case "missing-peer":
					require.NoError(t, os.Remove(filepath.Join(peer, "compose.yaml")))
				case "malformed-peer":
					require.NoError(t, os.WriteFile(filepath.Join(peer, "compose.yaml"), []byte("not: [valid"), 0600))
				case "malformed-primary":
					require.NoError(t, os.WriteFile(filepath.Join(path, ".env"), []byte("VALUE=\"unterminated\n"), 0600))
				}
				before := len(f.http.snapshot())
				err = identitySDKExecute(t, f, command)
				require.Empty(t, identitySDKEffects(f.http.snapshot()[before:]), "unknown or duplicate owner must issue zero mutating HTTP")
				t.Logf("change=%s kind=%s mutatingHTTP=%d", change, kind, len(identitySDKEffects(f.http.snapshot()[before:])))
				require.Error(t, err)
			})
		}
	}
}

func TestRuntimeExecutorIdentityAuxiliaryFallbackAndFreshValues(t *testing.T) {
	for _, source := range []string{"primary", "auxiliary", "primary-symlink", "auxiliary-symlink", "missing-aux", "malformed-aux", "invalid-aux", "ordered-aux", "include", "include-env", "repaired-aux", "stored-env-default"} {
		t.Run(source, func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			services := "services:\n  app:\n    image: registry.example.com/app:1\n    environment:\n      VALUE: ${VALUE:-fallback}\n    labels:\n      example.value: ${VALUE:-fallback}\n"
			if source == "include" {
				services = "include:\n  - extra.yaml\nservices: {}\n"
			}
			if source == "include-env" {
				services = "include:\n  - path: extra.yaml\n    env_file: include.env\nservices: {}\n"
			}
			wantName := "example_stack"
			prefix := "name: example_stack\n"
			if source == "stored-env-default" {
				wantName = "example-project"
				prefix = ""
			}
			path := f.project("example-project", "example-project", prefix+services)
			env := filepath.Join(path, ".env")
			edit, content := env, "VALUE=fresh\n"
			require.NoError(t, os.WriteFile(env, []byte("VALUE=stale\n"), 0600))
			want := "fresh"
			switch source {
			case "auxiliary", "auxiliary-symlink":
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_ENV_FILES=aux.env\n"), 0600))
				edit = filepath.Join(path, "aux.env")
				require.NoError(t, os.WriteFile(edit, []byte("VALUE=stale\n"), 0600))
			case "missing-aux", "malformed-aux", "invalid-aux", "ordered-aux":
				entries := "missing.env"
				if source == "invalid-aux" {
					entries = "../outside.env"
				}
				if source == "malformed-aux" || source == "ordered-aux" {
					entries = "bad.env"
					require.NoError(t, os.WriteFile(filepath.Join(path, "bad.env"), []byte("VALUE=\"unterminated\n"), 0600))
				}
				want = "fallback"
				if source == "ordered-aux" {
					entries = "valid.env," + entries
					require.NoError(t, os.WriteFile(filepath.Join(path, "valid.env"), []byte("VALUE=earlier\n"), 0600))
					want = "earlier"
				}
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_ENV_FILES="+entries+"\n"), 0600))
				edit = ""
			case "repaired-aux":
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_ENV_FILES=bad.env\n"), 0600))
				edit = filepath.Join(path, "bad.env")
				require.NoError(t, os.WriteFile(edit, []byte("VALUE=\"unterminated\n"), 0600))
			case "stored-env-default":
				require.NoError(t, os.WriteFile(env, []byte("COMPOSE_PROJECT_NAME=env_stack\nVALUE=fresh\n"), 0600))
				edit = ""
			case "include-env":
				require.NoError(t, os.WriteFile(filepath.Join(path, "extra.yaml"), []byte(servicesForIdentityInclude("${INCLUDED_VALUE}")), 0600))
				edit = filepath.Join(path, "include.env")
				require.NoError(t, os.WriteFile(edit, []byte("INCLUDED_VALUE=stale\n"), 0600))
				content = "INCLUDED_VALUE=fresh\n"
			case "include":
				edit = filepath.Join(path, "extra.yaml")
				require.NoError(t, os.WriteFile(edit, []byte(servicesForIdentityInclude("stale")), 0600))
				content = servicesForIdentityInclude("fresh")
			}
			if strings.HasSuffix(source, "symlink") {
				bytes, err := os.ReadFile(edit)
				require.NoError(t, err)
				require.NoError(t, os.Remove(edit))
				target := filepath.Join(t.TempDir(), "external.env")
				require.NoError(t, os.WriteFile(target, bytes, 0600))
				require.NoError(t, os.Symlink(target, edit))
				edit = target
			}
			_, err := f.projects.GetProjectDetails(t.Context(), "example-project", projecttypes.DetailsOptions{IncludeServiceConfigs: true})
			require.NoError(t, err)
			resolved, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			require.NoError(t, err)
			require.Equal(t, new(wantName), resolved.Target.Name)
			if edit != "" {
				identitySDKRewrite(t, edit, content)
			}
			f.http.setLabels(map[string]string{"com.docker.compose.project": wantName, "com.docker.compose.service": "app", "com.docker.compose.container-number": "1", "com.docker.compose.config-hash": "example-initial"})
			command := f.command(activity.RuntimeDeployProject, resolved.Target)
			command.Options.Deploy = &projecttypes.DeployOptions{PullPolicy: "never"}
			before := len(f.http.snapshot())
			require.NoError(t, identitySDKExecute(t, f, command))
			events := f.http.snapshot()[before:]
			require.NotEmpty(t, identitySDKEffects(events))
			labels := runtimeCreatedLabels(t, events)
			require.Equal(t, wantName, labels["com.docker.compose.project"])
			require.Equal(t, want, labels["example.value"])
			var creates, scopedLists int
			for _, event := range events {
				if event.Method == http.MethodPost && strings.HasSuffix(event.Path, "/containers/create") {
					var body struct {
						Env []string `json:"Env"`
					}
					require.NoError(t, json.Unmarshal([]byte(event.Body), &body))
					require.Contains(t, body.Env, "VALUE="+want)
					require.NotContains(t, body.Env, "VALUE=stale")
					creates++
				}
				if event.Method == http.MethodGet && strings.HasSuffix(event.Path, "/containers/json") {
					query, err := url.ParseQuery(event.Query)
					require.NoError(t, err)
					if filters := query.Get("filters"); strings.Contains(filters, "com.docker.compose.project=") {
						scopedLists++
						require.Contains(t, filters, "com.docker.compose.project="+wantName)
					}
				}
			}
			require.Equal(t, 1, creates)
			require.Positive(t, scopedLists, "actual SDK must filter by the fresh project name")
			t.Logf("source=%s name=%s mutatingHTTP=%d creates=%d scopedLists=%d value=%s", source, wantName, len(identitySDKEffects(events)), creates, scopedLists, want)
		})
	}
}

func servicesForIdentityInclude(value string) string {
	return "services:\n  app:\n    image: registry.example.com/app:1\n    environment:\n      VALUE: " + value + "\n    labels:\n      example.value: " + value + "\n"
}

func TestRuntimeExecutorIdentityArchivedParentAndStackGuard(t *testing.T) {
	f := newRuntimeServicesFixture(t)
	f.project("example-project", "example-project", "name: example_stack\n"+identitySDKServices)
	require.NoError(t, f.db.Model(&project.Project{}).Where("id = ?", "example-project").Update("is_archived", true).Error)
	f.http.setLabels(map[string]string{"com.docker.compose.project": "example_stack"})
	stack, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
	require.NoError(t, err)
	before := len(f.http.snapshot())
	require.Error(t, identitySDKExecute(t, f, f.command(activity.RuntimeDownProject, stack.Target)))
	require.Empty(t, identitySDKEffects(f.http.snapshot()[before:]))
	child, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "container", ID: runtimeContainerID})
	require.NoError(t, err)
	require.Equal(t, new("example-project"), child.Target.ParentProjectID)
	before = len(f.http.snapshot())
	require.NoError(t, identitySDKExecute(t, f, f.command(activity.RuntimeStartContainer, child.Target)))
	require.Len(t, identitySDKEffects(f.http.snapshot()[before:]), 1)
}

func TestRuntimeExecutorIdentityMalformedPrimaryHasZeroEffects(t *testing.T) {
	for _, warm := range []bool{false, true} {
		t.Run(map[bool]string{false: "cold", true: "warm-same-mtime"}[warm], func(t *testing.T) {
			f := newRuntimeServicesFixture(t)
			path := f.project("example-project", "example-project", "name: example_stack\n"+identitySDKServices)
			env := filepath.Join(path, ".env")
			if warm {
				require.NoError(t, os.WriteFile(env, []byte("VALUE=valid\n"), 0600))
			}
			resolved, err := f.adapter.ResolveRuntimeTarget(t.Context(), activity.RuntimeTarget{Kind: "stack", ID: "example-project"})
			require.NoError(t, err)
			if warm {
				identitySDKRewrite(t, env, "VALUE=\"unterminated\n")
			} else {
				require.NoError(t, os.WriteFile(env, []byte("VALUE=\"unterminated\n"), 0600))
			}
			before := len(f.http.snapshot())
			err = identitySDKExecute(t, f, f.command(activity.RuntimeDeployProject, resolved.Target))
			require.Empty(t, identitySDKEffects(f.http.snapshot()[before:]))
			require.ErrorContains(t, err, "runtime.target-unavailable")
		})
	}
}
