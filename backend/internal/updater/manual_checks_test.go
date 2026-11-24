// Source: docklet-next/issues/595
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original manual check freshness, policy and activity producer proofs.

package updater

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestManualContainerCheckFreshHostScopedAndActivity(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES('example-host','Example Host','https://agent.example.com',1)").Error)
	f.s.deps.Activity = activity.NewActivityService(f.db, f.s.deps.Settings)
	calls := 0
	f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
		require.Equal(t, "example-host", env)
		return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "example-container", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(_ context.Context, env string, target updateTarget) (*projectDTO.UpdateInfo, error) {
		require.Equal(t, "example-host", env)
		require.Equal(t, "example-container", target.ContainerID)
		calls++
		return f.check, nil
	}
	for range 2 {
		out, err := (&UpdaterHandler{updaterService: f.s}).CheckContainerUpdate(context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet()), &dto.CheckContainerInput{EnvironmentID: "example-host", ContainerID: "example-container"})
		require.NoError(t, err)
		require.True(t, out.Body.Success)
		require.Equal(t, "v1.1.0", out.Body.Data.LatestVersion)
	}
	require.Equal(t, 2, calls, "each POST must call the fresh checker")
	var activities []activity.Activity
	require.NoError(t, f.db.Order("created_at").Find(&activities).Error)
	require.Len(t, activities, 2)
	for _, row := range activities {
		require.Equal(t, "example-host", row.EnvironmentID)
		require.Equal(t, "image_update_check", string(row.Type))
		require.Equal(t, "app", *row.ResourceName)
		require.Equal(t, "check_container_update", row.Metadata["action"])
		require.Equal(t, "success", string(row.Status))
	}
	row, err := f.p.store.Policy(t.Context(), "example-host", "example-registration", "app", "container")
	require.NoError(t, err)
	require.NotNil(t, row.CheckedAt)
	// Use the real activity service DTO, with stable public fixture identity/time.
	detail, err := f.s.deps.Activity.GetActivityDetail(t.Context(), "example-host", activities[0].ID, 50)
	require.NoError(t, err)
	fixture := detail.Activity
	fixture.ID = "example-update-check"
	fixture.CreatedAt = time.Date(2026, 11, 24, 0, 0, 0, 0, time.UTC)
	fixture.StartedAt = fixture.CreatedAt
	fixture.EndedAt = &fixture.CreatedAt
	fixture.UpdatedAt = &fixture.CreatedAt
	fixture.DurationMs = nil
	body, err := json.MarshalIndent(fixture, "", "  ")
	require.NoError(t, err)
	path := filepath.Join("..", "..", "..", "web", "src", "features", "containers", "testdata")
	// The checked-in web fixture must equal the current producer output;
	// UPDATE_WEB_FIXTURES=1 regenerates it.
	fixturePath := filepath.Join(path, "manual-update-activity.json")
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		require.NoError(t, os.WriteFile(fixturePath, append(body, '\n'), 0644))
	}
	checkedIn, err := os.ReadFile(fixturePath)
	require.NoError(t, err)
	require.JSONEq(t, string(checkedIn), string(body))
}

func TestManualContainerCheckRefusesForeignAndUnmanagedInstances(t *testing.T) {
	f := newPolicyFixture(t)
	calls := 0
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app", ContainerID: "locked"}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) {
		calls++
		return f.check, nil
	}
	for _, id := range []string{"foreign", "locked"} {
		_, err := f.s.CheckPolicyContainer(t.Context(), "0", id)
		var problem *imageDTO.OperationError
		require.ErrorAs(t, err, &problem)
	}
	require.Zero(t, calls)
}

func TestHostCheckFailureAndRemovalStayOnTarget(t *testing.T) {
	for _, mode := range []string{"offline", "removed", "removed-during-check", "reregistered-during-check"} {
		t.Run(mode, func(t *testing.T) {
			f := newPolicyFixture(t)
			require.NoError(t, f.db.Exec("INSERT INTO environments(id,name,api_url,enabled,created_at) VALUES('example-host','Host','https://agent.example.com',1,?)", time.Now().UTC()).Error)
			touched := []string{}
			f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
				touched = append(touched, env)
				require.Equal(t, "example-host", env)
				if mode == "offline" {
					return nil, imageDTO.Failure("agent.unavailable", 503)
				}
				return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "instance", Eligible: true}}, nil
			}
			f.p.ports.checkContainer = func(_ context.Context, env string, _ updateTarget) (*projectDTO.UpdateInfo, error) {
				touched = append(touched, env)
				if mode == "removed-during-check" {
					require.NoError(t, f.db.Exec("DELETE FROM environments WHERE id='example-host'").Error)
				}
				if mode == "reregistered-during-check" {
					require.NoError(t, f.db.Exec("UPDATE environments SET credential_revision=credential_revision+1 WHERE id='example-host'").Error)
				}
				return f.check, nil
			}
			if mode == "removed" {
				require.NoError(t, f.db.Exec("DELETE FROM environments WHERE id='example-host'").Error)
			}
			out, err := f.s.RunUpdateCheckHost(t.Context(), "example-host")
			require.NoError(t, err)
			require.NotEmpty(t, out.Targets)
			require.NotEqual(t, "succeeded", string(out.Status))
			for _, target := range out.Targets {
				require.Equal(t, "example-host", target.EnvironmentID)
				require.Equal(t, "failed", string(target.Status))
			}
			for _, env := range touched {
				require.Equal(t, "example-host", env)
			}
		})
	}
}

func TestContainerCheckRequiresAdministratorBeforeReadingTarget(t *testing.T) {
	for _, ps := range []*authz.PermissionSet{nil, authz.EnvironmentPermissionSet("example-host")} {
		ctx := t.Context()
		if ps != nil {
			ctx = context.WithValue(ctx, middleware.ContextKeyUserPermissions, ps)
		}
		_, err := (&UpdaterHandler{}).CheckContainerUpdate(ctx, &dto.CheckContainerInput{EnvironmentID: "example-host", ContainerID: "example-container"})
		var problem *imageDTO.OperationError
		require.ErrorAs(t, err, &problem)
		require.Equal(t, 403, problem.Status)
	}
}

func TestManualProjectCheckRecordsHostAndProjectName(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES('example-host','Example Host','https://agent.example.com',1)").Error)
	f.s.deps.Activity = activity.NewActivityService(f.db, f.s.deps.Settings)
	f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
		require.Equal(t, "example-host", env)
		return []updateTarget{{Kind: "stack", ID: "project-id", Name: "example-project", Eligible: true}}, nil
	}
	f.p.ports.check = func(_ context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
		require.Equal(t, "example-host", env)
		require.Equal(t, "project-id", target)
		return f.check, nil
	}
	result, err := f.s.CheckPolicyProject(t.Context(), "example-host", "project-id")
	require.NoError(t, err)
	require.True(t, result.HasUpdate)
	var row activity.Activity
	require.NoError(t, f.db.First(&row).Error)
	require.Equal(t, "example-host", row.EnvironmentID)
	require.Equal(t, "example-project", *row.ResourceName)
	require.Equal(t, "check_project_update", row.Metadata["action"])
	require.Equal(t, "image_update_check", string(row.Type))
}

func TestHostCheckLocalTargetIncludesStacksAndStandaloneContainers(t *testing.T) {
	f := newPolicyFixture(t)
	f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
		require.Equal(t, "0", env)
		return []updateTarget{{Kind: "stack", ID: "project", Eligible: true}, {Kind: "container", ID: "app", Name: "app", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(_ context.Context, env string, _ updateTarget) (*projectDTO.UpdateInfo, error) {
		require.Equal(t, "0", env)
		return f.check, nil
	}
	out, err := f.s.RunUpdateCheckHost(t.Context(), "0")
	require.NoError(t, err)
	require.Equal(t, "succeeded", string(out.Status))
	require.Len(t, out.Targets, 2)
	for _, target := range out.Targets {
		require.Equal(t, "0", target.EnvironmentID)
	}
}

// A project whose services are all built locally has nothing to check: the
// host check stays succeeded and a manual check answers without a failure.
func TestBuildOnlyProjectDoesNotFailChecks(t *testing.T) {
	f := newPolicyFixture(t)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "stack", ID: "build-only", Name: "build-only", Eligible: true}, {Kind: "stack", ID: "project", Name: "project", Eligible: true}}, nil
	}
	f.p.ports.check = func(_ context.Context, _ string, target string) (*projectDTO.UpdateInfo, error) {
		if target == "build-only" {
			return &projectDTO.UpdateInfo{Status: "unknown", ImageCount: 0}, nil
		}
		return f.check, nil
	}
	out, err := f.s.RunUpdateCheckHost(t.Context(), "0")
	require.NoError(t, err)
	require.Equal(t, "succeeded", string(out.Status))
	require.Len(t, out.Targets, 2)
	for _, target := range out.Targets {
		require.Equal(t, "succeeded", string(target.Status), target.ID)
	}
	result, err := f.s.CheckPolicyProject(t.Context(), "0", "build-only")
	require.NoError(t, err)
	require.False(t, result.HasUpdate)
	// An image that cannot be read still fails the check.
	f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
		return &projectDTO.UpdateInfo{Status: "unknown", ImageCount: 1}, nil
	}
	_, err = f.s.CheckPolicyProject(t.Context(), "0", "build-only")
	require.Error(t, err)
}

// A project that became build-only replaces its earlier registry candidates with
// the empty observation, and a scheduled run applies nothing.
func TestBuildOnlyCheckClearsCandidatesAndAppliesNothing(t *testing.T) {
	for _, job := range []string{"update-check", "auto-update", "manual"} {
		t.Run(job, func(t *testing.T) {
			f := newPolicyFixture(t)
			one := f.save(t, "project", "scheduled")
			candidates, err := f.s.observeImagePolicy(t.Context(), f.p, one, f.check)
			require.NoError(t, err)
			require.Len(t, candidates, 1)
			f.check = &projectDTO.UpdateInfo{Status: "unknown", ImageCount: 0}
			if job == "manual" {
				_, err = f.s.CheckPolicyProject(t.Context(), "0", "project")
				require.NoError(t, err)
			} else {
				outcome, uncertain := f.s.runImagePolicyTarget(t.Context(), f.p, one, "example-run", job)
				require.False(t, uncertain)
				require.Equal(t, "succeeded", string(outcome.Status))
			}
			require.Zero(t, f.admissions)
			live, err := f.s.GetImagePolicy(t.Context(), "0", "project")
			require.NoError(t, err)
			require.Empty(t, live.Candidates)
		})
	}
}
