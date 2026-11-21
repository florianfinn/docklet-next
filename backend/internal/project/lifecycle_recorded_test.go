// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Recorded external lifecycle dependency and zero-effect regressions; M34 #548.

package project

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	compose "github.com/compose-spec/compose-go/v2/types"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/acfs"
)

func recordedLifecycle(t *testing.T, f *lifecycleFixture, inside ...bool) (context.Context, string, *life.Snapshot, *dto.LifecycleIntent) {
	t.Helper()
	path := filepath.Join(f.root, "recorded.env")
	if len(inside) > 0 && inside[0] {
		path = filepath.Join(f.project.Path, "recorded.env")
	}
	require.NoError(t, os.WriteFile(path, []byte("UNUSED=original\n"), 0600))
	labels := map[string]string{"com.docker.compose.project.working_dir": f.project.Path, "com.docker.compose.project.config_files": filepath.Join(f.project.Path, "compose.yaml"), projects.EnvironmentFileLabel: path}
	ctx := projects.WithComposeContainerLabels(t.Context(), func(context.Context) ([]map[string]string, error) { return []map[string]string{labels}, nil })
	_, snapshot, err := f.service.lifecycleCurrent(ctx, f.project.ID, f.registration)
	require.NoError(t, err)
	snapshot.RunID = uuid.NewString()
	require.NoError(t, f.service.lifecycleStore().SaveBaseline(ctx, snapshot))
	_, revision, err := f.service.lifecycleStore().Baseline(ctx, snapshot)
	require.NoError(t, err)
	intent := &dto.LifecycleIntent{Request: dto.LifecycleRequest{ExpectedRevision: snapshot.SavedRevision}, TargetIdentity: snapshot.TargetIdentity, SavedRevision: snapshot.SavedRevision, ObservedRevision: lifecycleObserved(snapshot), BaselineRevision: revision, Selections: []dto.ImageSelection{{Service: "app", SourceReference: "registry.example.com/app:1.0.0", Reference: "registry.example.com/app:1.1.0", Digest: lifecycleDigest, ImageID: lifecycleNewID, Platform: "linux/amd64", Kind: "version"}}}
	return ctx, path, snapshot, intent
}
func recordedWriteCounter(t *testing.T, path string) *int {
	t.Helper()
	count := new(int)
	original := lifecycleWriteFile
	t.Cleanup(func() { lifecycleWriteFile = original })
	lifecycleWriteFile = func(ctx context.Context, root, rel string, body []byte, opts acfs.WriteOptions) error {
		if filepath.Clean(filepath.Join(root, rel)) == path || filepath.Clean(root+rel) == path {
			*count++
		}
		return original(ctx, root, rel, body, opts)
	}
	return count
}
func TestRecordedLifecycleRevisionAndBaselineDrift(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx, path, snapshot, _ := recordedLifecycle(t, f)
	require.Len(t, snapshot.External, 1)
	require.Equal(t, life.Hash([]any{snapshot.Files, snapshot.Model, snapshot.External}), snapshot.SavedRevision)
	require.NotEqual(t, life.Hash([]any{snapshot.Files, snapshot.Model}), snapshot.SavedRevision)
	require.Equal(t, path, snapshot.External[0].Path)
	for _, file := range snapshot.Files {
		require.NotEqual(t, "recorded.env", file.Path)
		require.NotEqual(t, path, file.Path)
		require.NotEqual(t, ".env", file.Path)
	}
	require.NoError(t, os.WriteFile(path, []byte("UNUSED=changed\n"), 0600))
	_, current, err := f.service.lifecycleCurrent(ctx, f.project.ID, f.registration)
	require.NoError(t, err)
	require.NotEqual(t, snapshot.SavedRevision, current.SavedRevision, "unused recorded bytes bind the revision")
	status, err := f.service.ImageBaselineStatus(ctx, f.project.ID, f.registration)
	require.NoError(t, err)
	require.False(t, status.Available)
	require.Equal(t, "update.config-drift", status.Code)
	// The drift answer is a complete status that every consumer accepts.
	require.True(t, status.Valid())
	require.Equal(t, current.SavedRevision, status.SavedRevision)
	require.Equal(t, lifecycleObserved(current), status.ObservedRevision)
	requireDriftFixtureShape(t, status)
	require.NoError(t, os.Remove(path))
	_, err = f.service.ImageBaselineStatus(ctx, f.project.ID, f.registration)
	lifecycleCode(t, err, "update.snapshot-unavailable")
	_, err = f.service.PrepareImageLifecycle(ctx, f.project.ID, f.registration, dto.LifecycleRequest{ExpectedRevision: snapshot.SavedRevision})
	lifecycleCode(t, err, "update.config-drift")
}
func TestRecordedLifecycleEffectBarriers(t *testing.T) {
	for _, phase := range []string{"admission-effect", "pull-tag", "write", "restart", "restart-late", "restore", "recovery"} {
		t.Run(phase, func(t *testing.T) {
			f := newLifecycleFixture(t)
			ctx, path, snapshot, intent := recordedLifecycle(t, f)
			writes := recordedWriteCounter(t, path)
			change := func() { require.NoError(t, os.WriteFile(path, []byte("UNUSED=changed\n"), 0600)) }
			switch phase {
			case "admission-effect":
				change()
				_, err := f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, intent, user.Actor{ID: "operator"})
				lifecycleCode(t, err, "update.target-changed")
			case "pull-tag":
				tagsBeforeChange := 0
				f.onPull = func() { tagsBeforeChange = f.tagWrites; change() }
				_, err := f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, intent, user.Actor{ID: "operator"})
				lifecycleCode(t, err, "update.target-changed")
				// One point retention pin precedes the pull; no new run or target tags.
				require.Positive(t, tagsBeforeChange)
				require.Equal(t, tagsBeforeChange, f.tagWrites)
			case "write":
				desired, err := lifecycleVersionFiles(snapshot, intent.Selections)
				require.NoError(t, err)
				change()
				err = f.service.lifecycleWrite(ctx, f.project, snapshot, desired, user.Actor{ID: "operator"})
				lifecycleCode(t, err, "update.target-changed")
			case "restart":
				change()
				err := f.service.deployLifecycle(ctx, snapshot, false, snapshot)
				lifecycleCode(t, err, "update.target-changed")
			case "restart-late":
				changed := false
				f.onInspect = func(string) {
					if !changed {
						changed = true
						change()
					}
				}
				err := f.service.deployLifecycle(ctx, snapshot, false, snapshot)
				require.True(t, changed, "external edit must occur after the initial deploy read")
				lifecycleCode(t, err, "update.target-changed")
			case "restore":
				point, err := f.service.lifecycleStore().Prepare(ctx, snapshot, uuid.NewString(), "", "settled")
				require.NoError(t, err)
				require.NoError(t, f.service.lifecyclePinPoint(ctx, point, snapshot))
				change()
				_, current, err := f.service.lifecycleCurrent(ctx, f.project.ID, f.registration)
				require.NoError(t, err)
				_, err = f.service.PrepareImageLifecycle(ctx, f.project.ID, f.registration, dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
				lifecycleCode(t, err, "update.target-changed")
			case "recovery":
				run := uuid.NewString()
				snapshot.RunID = run
				point, err := f.service.lifecycleStore().Prepare(ctx, snapshot, run, "", "settled")
				require.NoError(t, err)
				require.NoError(t, f.service.lifecyclePinPoint(ctx, point, snapshot))
				result := &dto.LifecycleResult{RunID: run, RestorePointID: point.ID}
				point.Available = true
				change()
				err = f.service.recoverOwnPoint(ctx, run, f.registration, &lifecycleExecution{point: point}, snapshot, user.Actor{ID: "operator"}, result)
				require.NoError(t, err)
				require.NotNil(t, result.Recovery)
				require.Equal(t, "update.target-changed", result.Recovery.Code)
			}
			require.Equal(t, 0, *writes, "external paths must never enter a writable snapshot")
			require.Equal(t, "UNUSED=changed\n", readRecordedLifecycleFile(t, path))
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
			require.Zero(t, f.deploys)
			if phase != "pull-tag" {
				require.Zero(t, f.pulls)
			}
			require.Equal(t, lifecycleOldID, f.tags["registry.example.com/app:1.0.0"])
		})
	}
}
func readRecordedLifecycleFile(t *testing.T, path string) string {
	t.Helper()
	body, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(body)
}
func TestRecordedLifecycleUpdateAndRestoreNeverWriteExternal(t *testing.T) {
	for _, inside := range []bool{false, true} {
		t.Run(map[bool]string{false: "outside-stack", true: "inside-stack"}[inside], func(t *testing.T) {
			f := newLifecycleFixture(t)
			ctx, path, snapshot, intent := recordedLifecycle(t, f, inside)
			writes := recordedWriteCounter(t, path)
			run := uuid.NewString()
			result, err := f.service.ExecuteImageLifecycle(ctx, f.project.ID, run, f.registration, intent, user.Actor{ID: "operator"})
			require.NoError(t, err)
			require.True(t, result.Verified)
			point := lifecyclePoints(t, f)[0]
			_, current, err := f.service.lifecycleCurrent(ctx, f.project.ID, f.registration)
			require.NoError(t, err)
			restore, err := f.service.PrepareImageLifecycle(ctx, f.project.ID, f.registration, dto.LifecycleRequest{PointID: point.ID, ExpectedRevision: current.SavedRevision, ReplaceConfiguration: true})
			require.NoError(t, err)
			result, err = f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, restore, user.Actor{ID: "operator"})
			require.NoError(t, err)
			require.True(t, result.Verified)
			require.Zero(t, *writes)
			require.Equal(t, "UNUSED=original\n", readRecordedLifecycleFile(t, path))
			require.Equal(t, snapshot.External, current.External)
			require.Equal(t, lifecycleOriginal, lifecycleFile(t, f, "compose.yaml"))
		})
	}
}
func TestRecordedProjectionAllFields(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx, path, _, _ := recordedLifecycle(t, f)
	source := `name: example-stack
services:
  app:
    image: registry.example.com/app:${TAG}
    environment:
      VALUE: ${VALUE}
    command: [echo, '${VALUE}']
    entrypoint: ['${VALUE}']
    labels:
      example.value: ${VALUE}
    healthcheck:
      test: [CMD, '${VALUE}']
    volumes:
      - ${VALUE}:/example
`
	require.NoError(t, os.WriteFile(path, []byte("VALUE=/srv/example/recorded-sentinel\nTAG=1.0.0\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	model, _, err := f.service.loadComposeProjectForProject(ctx, f.project, nil)
	require.NoError(t, err)
	projected, err := projects.ProjectServiceProjection(ctx, model, f.root, nil)
	require.NoError(t, err)
	service := projected.Services["app"]
	require.Equal(t, "${VALUE}", *service.Environment["VALUE"])
	require.Contains(t, []string(service.Command), "${VALUE}")
	require.Contains(t, []string(service.Entrypoint), "${VALUE}")
	require.Equal(t, "${VALUE}", service.Labels["example.value"])
	require.Contains(t, []string(service.HealthCheck.Test), "${VALUE}")
	require.Contains(t, service.Volumes[0].Source, "${VALUE}")
	require.Equal(t, "registry.example.com/app:1.0.0", service.Image)
	require.Equal(t, "/srv/example/recorded-sentinel", *model.Services["app"].Environment["VALUE"], "executable model stays interpolated")
	// The project API's actual services[] producer must also use the copy.
	details := projectDetailsForRecordedProjection(ctx, f)
	require.Len(t, details, 1)
	require.Equal(t, "${VALUE}", *details[0].Environment["VALUE"])
	require.NotContains(t, strings.Join(details[0].Command, " "), "recorded-sentinel")
}
func projectDetailsForRecordedProjection(ctx context.Context, f *lifecycleFixture) []compose.ServiceConfig {
	var result projectDTO.Details
	f.service.enrichComposeDetails(ctx, f.project, projectDTO.DetailsOptions{IncludeServiceConfigs: true}, &result)
	return result.Services
}

func TestRecordedValidationDoesNotCreateConventionalEnv(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx, path, _, _ := recordedLifecycle(t, f)
	require.NoError(t, os.Remove(filepath.Join(f.project.Path, ".env")))
	content := lifecycleOriginal + "# edited definition\n"
	require.NoError(t, f.service.persistUpdatedProjectFiles(ctx, f.project, f.root, &content, nil, nil))
	require.NoFileExists(t, filepath.Join(f.project.Path, ".env"), "recorded inputs replace the conventional env file during validation and save")
	require.Equal(t, "UNUSED=original\n", readRecordedLifecycleFile(t, path))
	_, _, err := f.service.loadComposeProjectForProject(ctx, f.project, nil)
	require.NoError(t, err)
}

func TestRecordedProjectionTypedFieldsStayReadableAndValueFree(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx, path, _, _ := recordedLifecycle(t, f)
	source := `name: example-stack
services:
  app:
    image: registry.example.com/app:${TAG}
    build: .
    ports: ["${PORT}:80"]
    cpus: ${CPUS}
    environment:
      VALUE: ${VALUE}
    healthcheck:
      test: [CMD, "true"]
      interval: ${INTERVAL}
    deploy:
      replicas: ${REPL}
`
	require.NoError(t, os.WriteFile(path, []byte("VALUE=recorded-sentinel\nTAG=1.0.0\nPORT=18443\nCPUS=0.5\nINTERVAL=17s\nREPL=3\n"), 0600))
	require.NoError(t, os.WriteFile(filepath.Join(f.project.Path, "compose.yaml"), []byte(source), 0600))
	model, _, err := f.service.loadComposeProjectForProject(ctx, f.project, nil)
	require.NoError(t, err, "precondition: the executable load succeeds")
	projected, err := projects.ProjectServiceProjection(ctx, model, f.root, nil)
	require.NoError(t, err, "a typed ${VAR} field never makes the stack unreadable")
	service := projected.Services["app"]
	require.Equal(t, "registry.example.com/app:1.0.0", service.Image)
	require.NotNil(t, service.Build, "build presence survives")
	body, err := json.Marshal(projected)
	require.NoError(t, err)
	for _, value := range []string{"recorded-sentinel", "18443", "17s"} {
		require.NotContains(t, string(body), value)
	}
	details := projectDetailsForRecordedProjection(ctx, f)
	require.Len(t, details, 1, "services[] stays available")
	require.Equal(t, "app", details[0].Name)
}

func TestRecordedFailedRebuildRollsBackAfterExternalChange(t *testing.T) {
	f := newLifecycleFixture(t)
	ctx, path, _, intent := recordedLifecycle(t, f)
	writes := recordedWriteCounter(t, path)
	rebuild := *intent
	rebuild.Selections = []dto.ImageSelection{intent.Selections[0]}
	rebuild.Selections[0].Kind, rebuild.Selections[0].Reference = "rebuild", "registry.example.com/app:1.0.0"
	f.up = func(context.Context, *compose.Project) error {
		require.NoError(t, os.WriteFile(path, []byte("UNUSED=changed\n"), 0600))
		return errors.New("synthetic deployment failure")
	}
	_, err := f.service.ExecuteImageLifecycle(ctx, f.project.ID, uuid.NewString(), f.registration, &rebuild, user.Actor{ID: "operator"})
	require.Error(t, err)
	var uncertain *dto.UncertainImageEffect
	require.False(t, errors.As(err, &uncertain), "the compensating re-tag reads no env input, so it is not gated: %v", err)
	f.mu.Lock()
	tag := f.tags["registry.example.com/app:1.0.0"]
	f.mu.Unlock()
	require.Equal(t, lifecycleOldID, tag, "the previous image is tagged back")
	require.Zero(t, *writes)
}

// requireDriftFixtureShape binds the shared transport fixture (types/updater/testdata) to the producer's answer:
// the same fields, 64-hex revisions where the fixture has them, and the same literal values elsewhere.
func requireDriftFixtureShape(t *testing.T, status dto.BaselineStatus) {
	t.Helper()
	var fixture struct{ Fixed struct{ Data map[string]any } }
	body, err := os.ReadFile("../../../types/updater/testdata/drift-baseline.json")
	require.NoError(t, err)
	require.NoError(t, json.Unmarshal(body, &fixture))
	encoded, err := json.Marshal(status)
	require.NoError(t, err)
	var produced map[string]any
	require.NoError(t, json.Unmarshal(encoded, &produced))
	shape := func(m map[string]any) map[string]any {
		out := map[string]any{}
		for k, v := range m {
			if s, ok := v.(string); ok && len(s) == 64 && strings.Trim(s, "0123456789abcdef") == "" {
				v = "hex64"
			}
			out[k] = v
		}
		return out
	}
	require.Equal(t, shape(fixture.Fixed.Data), shape(produced))
}
