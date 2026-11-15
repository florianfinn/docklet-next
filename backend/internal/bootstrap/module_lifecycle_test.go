// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original persisted module admission, deletion, override and bootstrap proofs.

package bootstrap

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/types/features"
	moduleDTO "github.com/florianfinn/docklet-next/types/modules"
	schedulerDTO "github.com/florianfinn/docklet-next/types/scheduler"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func m27PersistedHooks(t *testing.T, db *database.DB, m *modules.Service) {
	t.Helper()
	// Both fixture tables are owned by the test module; the second row is unrelated.
	// The unfinished record is deliberately stored, including across service recreation.
	require.NoError(t, db.Exec("CREATE TABLE module_records (id TEXT PRIMARY KEY, module TEXT, unfinished BOOLEAN)").Error)
	require.NoError(t, m.Register(features.Proxy, modules.Hooks{Unfinished: func(ctx context.Context) (string, error) {
		var id string
		err := db.WithContext(ctx).Raw("SELECT id FROM module_records WHERE module = 'proxy' AND unfinished = true ORDER BY id LIMIT 1").Scan(&id).Error
		return id, err
	}, DeleteData: func(ctx context.Context) error {
		return db.WithContext(ctx).Exec("DELETE FROM module_records WHERE module = 'proxy'").Error
	}}))
}

func TestM27M4SettingsAdmissionFence(t *testing.T) {
	db := m27DB(t)
	s := m27Settings(t, db)
	m := modules.New(s)
	m27PersistedHooks(t, db, m)
	require.NoError(t, s.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	inside, release := make(chan struct{}), make(chan struct{})
	admitted := make(chan error, 1)
	go func() {
		admitted <- m.Admit(t.Context(), features.Proxy, func(ctx context.Context) error {
			if err := db.WithContext(ctx).Exec("INSERT INTO module_records VALUES ('apply-1','proxy',true)").Error; err != nil {
				return err
			}
			close(inside)
			<-release
			return nil
		})
	}()
	<-inside
	disabled := make(chan error, 1)
	started := make(chan struct{})
	go func() {
		close(started)
		_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
		disabled <- err
	}()
	<-started
	select {
	case err := <-disabled:
		t.Fatalf("settings write crossed the held admission interval: %v", err)
	default:
	}
	close(release)
	require.NoError(t, <-admitted)
	var busy *moduleDTO.Problem
	require.ErrorAs(t, <-disabled, &busy)
	require.Equal(t, "module-busy", busy.Code)
	require.Equal(t, "apply-1", busy.Operation)
	require.True(t, s.IsFeatureEnabled(t.Context(), features.Proxy))
	require.NoError(t, db.Exec("UPDATE module_records SET unfinished = false WHERE id = 'apply-1'").Error)
	_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
	require.NoError(t, err)
	called := false
	err = m.Admit(t.Context(), features.Proxy, func(context.Context) error { called = true; return nil })
	require.Error(t, err)
	require.False(t, called, "no persisted reservation may follow disable")
}

func TestM27M6ModuleDataDeletion(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			x := newM27ModuleFixture(t, mode, false)
			m27PersistedHooks(t, x.f.db, x.manager)
			require.NoError(t, x.f.db.Exec("INSERT INTO module_records VALUES ('saved','proxy',false),('other','unrelated',false)").Error)
			sentinel := filepath.Join(t.TempDir(), "proxy-config.yml")
			require.NoError(t, os.WriteFile(sentinel, []byte("keep host configuration"), 0600))
			require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
			out := x.request("DELETE", "/api/modules/proxy/data", "")
			require.Equal(t, 409, out.Code)
			require.Contains(t, out.Body.String(), "module-on")
			var count int64
			require.NoError(t, x.f.db.Table("module_records").Where("module = ?", "proxy").Count(&count).Error)
			require.EqualValues(t, 1, count)
			require.NoError(t, x.hubSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
			require.NoError(t, x.f.db.Exec("UPDATE module_records SET unfinished = true WHERE id = 'saved'").Error)
			out = x.request("DELETE", "/api/modules/proxy/data", "")
			require.Equal(t, 409, out.Code)
			require.Contains(t, out.Body.String(), "module-busy")
			require.Contains(t, out.Body.String(), "saved")
			require.NoError(t, x.f.db.Exec("UPDATE module_records SET unfinished = false WHERE id = 'saved'").Error)
			// A reachable agent with no registered deleter succeeds and is not pending.
			out = x.request("DELETE", "/api/modules/proxy/data", "")
			require.Equal(t, 200, out.Code, out.Body.String())
			require.Contains(t, out.Body.String(), `"pendingCleanup":[]`)
			// A reachable agent's own refusal is the pending reason, not unavailability.
			require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
			out = x.request("DELETE", "/api/modules/proxy/data", "")
			require.Equal(t, 200, out.Code, out.Body.String())
			require.Contains(t, out.Body.String(), `"reason":"module-on"`)
			require.NoError(t, x.agentSettings.UpdateSetting(t.Context(), features.ProxySettingKey, "false"))
			// The hub-local host form is absent; deletion always runs through the management route.
			require.NoError(t, x.f.db.Exec("INSERT INTO module_records VALUES ('hub-local','proxy',false)").Error)
			out = x.request("DELETE", "/api/environments/0/modules/proxy/data", "")
			require.Equal(t, 404, out.Code, out.Body.String())
			require.Contains(t, out.Body.String(), "API endpoint not found")
			require.NoError(t, x.f.db.Table("module_records").Where("id = ?", "hub-local").Count(&count).Error)
			require.EqualValues(t, 1, count)
			require.NoError(t, x.f.db.Exec("INSERT INTO module_records VALUES ('saved-again','proxy',false)").Error)
			x.online.Store(false)
			out = x.request("DELETE", "/api/modules/proxy/data", "")
			require.Equal(t, 200, out.Code, out.Body.String())
			require.Contains(t, out.Body.String(), `"id":"example"`)
			require.NoError(t, x.f.db.Table("module_records").Where("module = ?", "proxy").Count(&count).Error)
			require.Zero(t, count)
			require.NoError(t, x.f.db.Table("module_records").Where("module = ?", "unrelated").Count(&count).Error)
			require.EqualValues(t, 1, count)
			bytes, err := os.ReadFile(sentinel)
			require.NoError(t, err)
			require.Equal(t, "keep host configuration", string(bytes))
			out = x.request("GET", "/api/modules", "")
			require.Equal(t, 200, out.Code)
			require.Contains(t, out.Body.String(), `"enabled":false`)
		})
	}
}

func TestM27M9ModuleEnvironmentCannotOverrideHub(t *testing.T) {
	for _, mode := range []string{"hub", "agent", "edge-agent"} {
		t.Run(mode, func(t *testing.T) {
			t.Setenv("MODULE_PROXY_ENABLED", "true")
			t.Setenv("AGENT_MODE", map[bool]string{true: "true", false: "false"}[mode == "agent"])
			t.Setenv("EDGE_AGENT", map[bool]string{true: "true", false: "false"}[mode == "edge-agent"])
			s := m27Settings(t, m27DB(t))
			require.False(t, s.IsFeatureEnabled(t.Context(), features.Proxy))
			require.False(t, s.IsEnvOverrideActive(features.ProxySettingKey))
			require.Contains(t, s.ListSettings(settings.SettingVisibilityPublic), settings.SettingVariable{Key: features.ProxySettingKey, Value: "false"})
			_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("true")})
			require.NoError(t, err)
			t.Setenv("MODULE_PROXY_ENABLED", "false")
			require.NoError(t, s.LoadDatabaseSettings(t.Context()))
			require.True(t, s.IsFeatureEnabled(t.Context(), features.Proxy))
		})
	}
	x := newM27ModuleFixture(t, "direct", false)
	out := x.request("PUT", "/api/environments/example/settings", `{"moduleProxyEnabled":"true"}`)
	require.Equal(t, 400, out.Code, "hub must reject per-host module writes")
	require.False(t, x.agentSettings.IsFeatureEnabled(t.Context(), features.Proxy))
}

type m27ModuleJob struct{ s *settings.SettingsService }

func (j m27ModuleJob) Name() string                    { return "test-module-proxy" }
func (j m27ModuleJob) Schedule(context.Context) string { return "0 0 0 * * *" }
func (j m27ModuleJob) ShouldSchedule(ctx context.Context) bool {
	return j.s.IsFeatureEnabled(ctx, features.Proxy)
}
func (j m27ModuleJob) Run(context.Context) (schedulerDTO.Outcome, error) {
	return schedulerDTO.Outcome{Status: schedulerDTO.Succeeded}, nil
}
func TestM27M11FreshAndUpgradeDefaultOff(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		t.Run(map[bool]string{false: "fresh", true: "upgrade"}[upgrade], func(t *testing.T) {
			db := m27DB(t)
			if upgrade {
				s := m27Settings(t, db)
				require.NoError(t, s.UpdateSetting(t.Context(), "hubName", "Existing hub"))
				require.NoError(t, db.Where("key = ?", features.ProxySettingKey).Delete(&settings.SettingVariable{}).Error)
			}
			s := m27Settings(t, db)
			assert.False(t, s.IsFeatureEnabled(t.Context(), features.Proxy))
			var stored settings.SettingVariable
			require.NoError(t, db.Where("key = ?", features.ProxySettingKey).First(&stored).Error)
			assert.Equal(t, "false", stored.Value)
			runtime := francistest.New(t)
			coordinator := runs.New(runtime.Service(), time.UTC)
			coordinator.SetExecutor(func(context.Context, schedulerDTO.Run) (schedulerDTO.Outcome, error) {
				return schedulerDTO.Outcome{}, errors.New("disabled module ran")
			}, nil)
			require.NoError(t, coordinator.Register(runtime))
			require.NoError(t, runtime.Start(t.Context(), t.Context(), nil))
			t.Cleanup(func() { require.NoError(t, runtime.Stop(context.WithoutCancel(t.Context()))) })
			require.NoError(t, coordinator.Start(t.Context(), t.Context()))
			coordinator.Activate()
			t.Cleanup(func() { require.NoError(t, coordinator.Stop(context.WithoutCancel(t.Context()))) })

			js, err := scheduler.NewJobScheduler(t.Context(), coordinator, time.UTC)
			require.NoError(t, err)
			require.NoError(t, js.RegisterJob(m27ModuleJob{s}))
			require.NoError(t, js.StartScheduler(t.Context()))
			defer js.Stop(context.WithoutCancel(t.Context()))
			state, ok := js.GetJobRuntimeState("test-module-proxy")
			require.True(t, ok)
			require.False(t, state.Scheduled, "fresh and upgraded hubs install no module job")
		})
	}
}
