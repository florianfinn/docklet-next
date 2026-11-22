// Source: docklet-next/issues/571
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original repository check stage boundaries and recorded stage messages.

package backup

import (
	"context"
	"errors"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
)

func stageRecorder() (*[]string, func(string)) {
	seen := []string{}
	return &seen, func(name string) { seen = append(seen, name) }
}

func TestRepositoryCheckEngineStagesStopAtTheFailingOperation(t *testing.T) {
	for _, tc := range []struct {
		name       string
		initialize bool
		exit       int64
		want       []string
	}{
		{"existing repository", false, 0, []string{"open", "check"}},
		{"witnessed empty repository", true, 0, []string{"init", "open", "check"}},
		{"wrong key on existing repository", false, 1, []string{"open"}},
		{"empty repository init failure", true, 1, []string{"init"}},
		{"integrity check fails after a successful open", false, -1, []string{"open", "check"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := newRuntimePlan(t)
			f.exit = tc.exit
			if tc.exit < 0 {
				f.exit, f.exitFor = 0, map[string]int64{"check": 1}
			}
			ctx := f.targetContext("local")
			cli, err := f.service.deps.Docker.GetClient(ctx)
			require.NoError(t, err)
			repository, err := f.service.repositoryFor(ctx, cli, "archive", false)
			require.NoError(t, err)
			seen, stage := stageRecorder()
			_, err = f.service.deps.Engine.CheckRepository(ctx, cli, repository, "synthetic", tc.initialize, stage)
			require.Equal(t, tc.exit != 0, err != nil)
			require.Equal(t, tc.want, *seen)
			creates, _, _ := f.effects()
			initialised := false
			for _, created := range creates {
				initialised = initialised || (len(created.Cmd) > 0 && created.Cmd[0] == "init")
			}
			require.Equal(t, tc.initialize, initialised, "only a witnessed-empty directory receives init")
		})
	}
}

func TestRepositoryCheckServiceStagesPrecedeTheirOperations(t *testing.T) {
	t.Run("probe failure stops before the engine", func(t *testing.T) {
		f := newRuntimePlan(t)
		f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) {
			return "", "", errors.New("synthetic denied")
		}
		seen, stage := stageRecorder()
		_, err := f.service.checkRepository(f.targetContext("local"), "archive", stage)
		require.Error(t, err)
		require.Equal(t, []string{"connect", "save"}, *seen)
	})
	t.Run("mount witness failure already reached connect", func(t *testing.T) {
		f := newRuntimePlan(t)
		f.witness = "1\n1"
		seen, stage := stageRecorder()
		_, err := f.service.checkRepository(f.targetContext("mount"), "archive", stage)
		require.ErrorContains(t, err, "backup.target.not-mounted")
		require.Equal(t, []string{"connect"}, *seen)
	})
	t.Run("helper image failure already reached connect", func(t *testing.T) {
		f := newRuntimePlan(t)
		f.service.deps.HelperImage = func(context.Context, *client.Client) (string, error) {
			return "", errors.New("synthetic helper unavailable")
		}
		seen, stage := stageRecorder()
		_, err := f.service.checkRepository(f.targetContext("local"), "archive", stage)
		require.ErrorContains(t, err, "synthetic helper unavailable")
		require.Equal(t, []string{"connect"}, *seen)
	})
	t.Run("successful check reaches every stage in order", func(t *testing.T) {
		f := newRuntimePlan(t)
		f.service.deps.Exec = func(context.Context, string, string, []string) (string, string, error) {
			return "present", "", nil
		}
		seen, stage := stageRecorder()
		_, err := f.service.checkRepository(f.targetContext("local"), "archive", stage)
		require.NoError(t, err)
		require.Equal(t, []string{"connect", "save", "open", "check"}, *seen)
	})
}

func TestRepositoryCheckStageRecordsStepMessageAndOutputLine(t *testing.T) {
	f := newRuntimePlan(t)
	require.NoError(t, f.service.deps.DB.AutoMigrate(activity.Models()...))
	f.service.deps.Activity = activity.NewActivityService(f.service.deps.DB, nil)
	ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
	id, _ := activitylib.StartHandlerActivity(ctx, f.service.deps.Activity, "0", activitytypes.TypeResourceAction, "backup_target", "local", "local", &user.SystemUser, "Checking backup repository", "Checking backup repository", nil, false)
	require.NotEmpty(t, id)
	stage := f.service.repositoryCheckStage(ctx, id)
	stage("connect")
	stage("open")
	stage("check")
	f.service.repositoryCheckResult(ctx, id, 3)
	detail, err := f.service.deps.Activity.GetActivityDetail(ctx, "0", id, 50)
	require.NoError(t, err)
	steps := []string{}
	for _, entry := range detail.StepTrace {
		steps = append(steps, entry.Name)
	}
	require.Equal(t, []string{"connect", "open", "check"}, steps, "the result line stays on the check stage")
	texts := []string{}
	for _, line := range detail.Output {
		texts = append(texts, line.Text)
	}
	require.Equal(t, []string{repositoryCheckStages["connect"], repositoryCheckStages["open"], repositoryCheckStages["check"], "Repository holds 3 snapshots"}, texts)
}
