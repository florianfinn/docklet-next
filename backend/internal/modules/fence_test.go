// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original assertions that persisted unfinished reads, admissions racing a switch-off and the settings persist share the admission gate.

package modules

import (
	"context"
	"errors"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func newGateFixture(t *testing.T) (*database.DB, *settings.SettingsService, *Service) {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "gate.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	s, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, s.Stop(context.WithoutCancel(t.Context()))) })
	return db, s, New(s)
}

func TestM27M4UnfinishedSourceSharesAdmissionGate(t *testing.T) {
	_, s, m := newGateFixture(t)
	reads := 0
	require.NoError(t, m.Register(features.Proxy, Hooks{Unfinished: func(context.Context) (string, error) {
		reads++
		// A read outside the gate can race an admission committing its pending row.
		unlocked := m.gates[features.Proxy].TryLock()
		if unlocked {
			m.gates[features.Proxy].Unlock()
		}
		require.False(t, unlocked, "the persisted unfinished read must run inside the admission gate")
		return "", nil
	}}))
	_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
	require.NoError(t, err)
	require.Equal(t, 1, reads)
}

// An admission that arrives while a switch-off holds the gate must observe the
// committed off state and never run its commit.
func TestM27M4AdmissionDuringSwitchOffObservesOff(t *testing.T) {
	_, s, m := newGateFixture(t)
	_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("true")})
	require.NoError(t, err)
	inside, release := make(chan struct{}), make(chan struct{})
	require.NoError(t, m.Register(features.Proxy, Hooks{Unfinished: func(context.Context) (string, error) {
		close(inside)
		<-release
		return "", nil
	}}))
	switched := make(chan error, 1)
	go func() {
		_, err := s.UpdateSettings(context.WithoutCancel(t.Context()), settingsDTO.Update{ModuleProxyEnabled: new("false")})
		switched <- err
	}()
	<-inside
	var ran atomic.Bool
	admitted := make(chan error, 1)
	go func() {
		admitted <- m.Admit(context.WithoutCancel(t.Context()), features.Proxy, func(context.Context) error { ran.Store(true); return nil })
	}()
	select {
	case err := <-admitted:
		require.Failf(t, "admission finished while the switch-off held the gate", "err=%v ran=%v", err, ran.Load())
	case <-time.After(200 * time.Millisecond):
	}
	close(release)
	require.NoError(t, <-switched)
	err = <-admitted
	var problem *dto.Problem
	require.True(t, errors.As(err, &problem), "admission after switch-off must be refused, got %v", err)
	require.Equal(t, "feature-disabled", problem.Code)
	require.False(t, ran.Load(), "the pending-record commit must not run after switch-off")
}

// The gate stays held while the module key is persisted, so no admission can
// read the old value between the guard check and the commit.
func TestM27M4GateHeldDuringSettingsPersist(t *testing.T) {
	db, s, m := newGateFixture(t)
	_, err := s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("true")})
	require.NoError(t, err)
	require.NoError(t, m.Register(features.Proxy, Hooks{Unfinished: func(context.Context) (string, error) { return "", nil }}))
	var checking atomic.Bool
	var persists, unguarded atomic.Int32
	check := func(*gorm.DB) {
		if !checking.Load() {
			return
		}
		persists.Add(1)
		if m.gates[features.Proxy].TryLock() {
			m.gates[features.Proxy].Unlock()
			unguarded.Add(1)
		}
	}
	require.NoError(t, db.Callback().Update().Before("gorm:update").Register("m27:gate-check-update", check))
	require.NoError(t, db.Callback().Create().Before("gorm:create").Register("m27:gate-check-create", check))
	checking.Store(true)
	_, err = s.UpdateSettings(t.Context(), settingsDTO.Update{ModuleProxyEnabled: new("false")})
	checking.Store(false)
	require.NoError(t, err)
	require.Positive(t, persists.Load(), "the module key must be persisted")
	require.Zero(t, unguarded.Load(), "the admission gate must be held while the module key is persisted")
	require.False(t, m.Enabled(t.Context(), features.Proxy))
}
