// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package autoheal

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	nDTO "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func healStore(t *testing.T) (*Store, *time.Time) {
	t.Helper()
	g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	db := &database.DB{DB: g}
	require.NoError(t, db.AutoMigrate(activity.Models()...))
	require.NoError(t, db.AutoMigrate(&Restart{}, &event.Event{}, &notificationoutbox.Row{}))
	pool, err := g.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	now := time.Now().UTC()
	return &Store{DB: db, Config: &config.Config{}, Now: func() time.Time { return now }}, &now
}
func TestDurableLimitsCountFailuresAndDeduplicateWindowMarker(t *testing.T) {
	s, now := healStore(t)
	cfg := dto.Settings{Enabled: true, MaxRestarts: 2, WindowMinutes: 30}
	ctx := t.Context()
	for _, id := range []string{"one", "two"} {
		r, restart, err := s.Reserve(ctx, id, "container", "web", "app", Baseline{StartedAt: "before"}, cfg)
		require.NoError(t, err)
		require.True(t, restart)
		require.Equal(t, "attempting", r.Outcome)
		require.Nil(t, r.SettledAt)
		require.NoError(t, s.Settle(ctx, r.ID, "restart-failed"))
		require.NoError(t, s.Project(ctx, r.ID, true))
	}
	r, restart, err := s.Reserve(ctx, "three", "container", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.False(t, restart)
	require.Equal(t, "limit-reached", r.Outcome)
	require.NoError(t, s.Project(ctx, r.ID, true))
	r, _, err = s.Reserve(ctx, "four", "container", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.Nil(t, r)
	var n int64
	require.NoError(t, s.DB.Model(&Restart{}).Where("outcome='limit-reached'").Count(&n).Error)
	require.EqualValues(t, 1, n)
	*now = now.Add(31 * time.Minute)
	r, restart, err = s.Reserve(ctx, "five", "container", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.True(t, restart)
	require.NotNil(t, r)
	// Same run/container never acquires another attempt, even under a fresh window.
	duplicate, restart, err := s.Reserve(ctx, "five", "container", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.False(t, restart)
	require.Equal(t, r.ID, duplicate.ID)
}
func TestProjectionLiveVersusReplayOnHubAndAgent(t *testing.T) {
	for _, agent := range []bool{false, true} {
		for _, live := range []bool{false, true} {
			t.Run(map[bool]string{false: "hub", true: "agent"}[agent]+map[bool]string{false: "-replay", true: "-live"}[live], func(t *testing.T) {
				s, _ := healStore(t)
				s.Config.AgentMode = agent
				dispatches := 0
				s.Dispatch = func(ctx context.Context, p nDTO.DispatchRequest) error {
					dispatches++
					require.Equal(t, "restarted", p.AutoHeal.Outcome)
					var row Restart
					require.NoError(t, s.DB.First(&row).Error)
					require.NotNil(t, row.ProjectedAt, "agent dispatch happens after commit")
					return errors.New("controlled unreachable hub")
				}
				r, _, err := s.Reserve(t.Context(), "one", "container", "web", "app", Baseline{}, dto.Settings{MaxRestarts: 5, WindowMinutes: 30})
				require.NoError(t, err)
				require.NoError(t, s.Settle(t.Context(), r.ID, "restarted"))
				require.NoError(t, s.Project(t.Context(), r.ID, live))
				require.NoError(t, s.Replay(t.Context()))
				require.NoError(t, s.Project(t.Context(), r.ID, true))
				var activities []activity.Activity
				require.NoError(t, s.DB.Find(&activities).Error)
				require.Len(t, activities, 1)
				require.Equal(t, "container_auto_heal", string(activities[0].Type))
				var events []event.Event
				require.NoError(t, s.DB.Find(&events).Error)
				require.Len(t, events, 1)
				require.Equal(t, "0", *events[0].EnvironmentID)
				var n int64
				require.NoError(t, s.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
				if live && agent {
					require.Equal(t, 1, dispatches)
					require.Zero(t, n)
				} else if live {
					require.EqualValues(t, 1, n)
					require.Zero(t, dispatches)
				} else {
					require.Zero(t, n)
					require.Zero(t, dispatches)
				}
				// A changed pairing's live dispatch callback is never consulted by replay.
				s.Dispatch = func(context.Context, nDTO.DispatchRequest) error {
					t.Error("historical notification replayed under a new pairing")
					return nil
				}
				require.NoError(t, s.Replay(t.Context()))
			})
		}
	}
}
func TestProjectionOutboxFailureRollsBackMarkerAndProjections(t *testing.T) {
	s, _ := healStore(t)
	r, _, err := s.Reserve(t.Context(), "one", "container", "web", "app", Baseline{}, dto.Settings{MaxRestarts: 5, WindowMinutes: 30})
	require.NoError(t, err)
	require.NoError(t, s.Settle(t.Context(), r.ID, "restarted"))
	require.NoError(t, s.DB.Exec(`CREATE TRIGGER outbox_fault BEFORE INSERT ON notification_outbox BEGIN SELECT RAISE(ABORT,'controlled failure'); END`).Error)
	require.Error(t, s.Project(t.Context(), r.ID, true))
	require.NoError(t, s.DB.First(r, r.ID).Error)
	require.Nil(t, r.ProjectedAt)
	var n int64
	require.NoError(t, s.DB.Model(&activity.Activity{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, s.DB.Exec(`DROP TRIGGER outbox_fault`).Error)
	require.NoError(t, s.Replay(t.Context()))
	require.NoError(t, s.DB.Model(&activity.Activity{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, s.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
}
func TestRecoveryBaselineAndSettleFailureKeepAttemptsCounted(t *testing.T) {
	s, _ := healStore(t)
	r, _, err := s.Reserve(t.Context(), "one", "container", "web", "app", Baseline{StartedAt: "before", RestartCount: 1}, dto.Settings{MaxRestarts: 5, WindowMinutes: 30})
	require.NoError(t, err)
	require.NoError(t, s.DB.Exec(`CREATE TRIGGER settle_fault BEFORE UPDATE OF outcome ON auto_heal_restarts BEGIN SELECT RAISE(ABORT,'controlled failure'); END`).Error)
	require.Error(t, s.Settle(t.Context(), r.ID, "restarted"))
	require.NoError(t, s.DB.First(r, r.ID).Error)
	require.Equal(t, "attempting", r.Outcome)
	require.NoError(t, s.DB.Exec(`DROP TRIGGER settle_fault`).Error)
	require.NoError(t, s.Recover(t.Context(), func(context.Context, string) (Baseline, error) {
		return Baseline{StartedAt: "after", RestartCount: 1}, nil
	}))
	require.NoError(t, s.DB.First(r, r.ID).Error)
	require.Equal(t, "restarted", r.Outcome)
	var n int64
	require.NoError(t, s.DB.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
}

func TestAgentEventIdentitySurvivesStoreReset(t *testing.T) {
	ids := map[string]bool{}
	for reset := range 2 {
		s, now := healStore(t)
		*now = now.Add(time.Duration(reset) * time.Minute)
		s.Config.AgentMode = true
		s.Dispatch = func(_ context.Context, p nDTO.DispatchRequest) error {
			require.Regexp(t, `^[A-Za-z0-9:_-]{1,64}$`, p.AutoHeal.EventID)
			ids[p.AutoHeal.EventID] = true
			return nil
		}
		r, _, err := s.Reserve(t.Context(), "run", "container", "web", "app", Baseline{}, dto.Settings{MaxRestarts: 5, WindowMinutes: 30})
		require.NoError(t, err)
		require.EqualValues(t, 1, r.ID, "a recreated store restarts row IDs")
		require.NoError(t, s.Settle(t.Context(), r.ID, "restarted"))
		require.NoError(t, s.Project(t.Context(), r.ID, true))
	}
	require.Len(t, ids, 2)
}
