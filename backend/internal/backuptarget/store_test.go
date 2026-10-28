// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original encrypted authority, immutable-location and retirement tests.

package backuptarget

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

func testStore(t *testing.T) *Store {
	t.Helper()
	g, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, g.AutoMigrate(&Target{}, &Check{}, &setting{}))
	pool, err := g.DB()
	require.NoError(t, err)
	pool.SetMaxOpenConns(1)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	return &Store{DB: &database.DB{DB: g}, SessionCurrent: func(context.Context, *gorm.DB) error { return nil }}
}
func synced(t *testing.T, id string) dto.SyncedTarget {
	t.Helper()
	key, err := backup.GenerateRecoveryKey()
	require.NoError(t, err)
	return dto.SyncedTarget{TargetLocation: dto.TargetLocation{Kind: "smb", Server: "nas.example.com", Share: "backups"}, ID: id, Name: id, Secret: strings.Repeat("p", 32), RecoveryKey: key, ConfigVersion: 1}
}
func envelope(t *testing.T, authority, host string, version int64, targets ...dto.SyncedTarget) dto.TargetSet {
	t.Helper()
	hash, err := ContentHash(targets)
	require.NoError(t, err)
	return dto.TargetSet{Authority: authority, HubHostID: host, SetVersion: version, ContentHash: hash, Targets: targets}
}
func code(t *testing.T, err error, want string) {
	t.Helper()
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, want, p.Code)
}

func TestSyncAuthorityOrderingAndRetainedKeys(t *testing.T) {
	s := testStore(t)
	target := synced(t, "first")
	set := envelope(t, "authority-a", "host-a", 10, target)
	_, err := s.Apply(t.Context(), set)
	require.NoError(t, err)
	_, err = s.Apply(t.Context(), set)
	require.NoError(t, err)
	_, err = s.Apply(t.Context(), envelope(t, "authority-a", "host-a", 9, target))
	code(t, err, "backup.target.stale-set")
	other := target
	other.Name = "changed"
	_, err = s.Apply(t.Context(), envelope(t, "authority-a", "host-a", 10, other))
	code(t, err, "backup.target.stale-set")
	_, err = s.Apply(t.Context(), envelope(t, "authority-a", "host-a", 11))
	require.NoError(t, err)
	_, err = s.Resolve(t.Context(), target.ID, "host-a", true)
	code(t, err, "backup.target.retired")
	held, err := s.Resolve(t.Context(), target.ID, "host-a", false)
	require.NoError(t, err)
	require.Equal(t, target.RecoveryKey, held.RecoveryKey)
	_, err = s.Apply(t.Context(), set)
	code(t, err, "backup.target.stale-set")
	// A restored hub starts a new authority at 1, independent of clocks/high water.
	_, err = s.Apply(t.Context(), envelope(t, "authority-b", "host-a", 1, target))
	require.NoError(t, err)
	_, err = s.Resolve(t.Context(), target.ID, "host-a", true)
	require.NoError(t, err)
	// Re-pairing the same agent DB to a new host resets the recipient namespace.
	_, err = s.Apply(t.Context(), envelope(t, "authority-b", "host-b", 1, target))
	require.NoError(t, err)
	host, err := readSetting(s.DB.DB, "hubHostId")
	require.NoError(t, err)
	require.Equal(t, "host-b", host)
	// Constructing a new Store preserves durable authority state after restart.
	restarted := &Store{DB: s.DB, SessionCurrent: s.SessionCurrent}
	_, err = restarted.Apply(t.Context(), envelope(t, "authority-b", "host-b", 1, other))
	code(t, err, "backup.target.stale-set")
}
func TestSyncRetainsImmutableLocationAcrossAuthorities(t *testing.T) {
	s := testStore(t)
	target := synced(t, "fixed")
	_, err := s.Apply(t.Context(), envelope(t, "newer", "host", 100, target))
	require.NoError(t, err)
	redirected := target
	redirected.Share = "other"
	receipt, err := s.Apply(t.Context(), envelope(t, "restored", "host", 1, redirected))
	require.NoError(t, err)
	require.Equal(t, []string{target.ID}, receipt.Conflicts)
	row, err := s.Resolve(t.Context(), target.ID, "host", false)
	require.NoError(t, err)
	require.Equal(t, target.Share, row.Share)
	replay, err := s.Apply(t.Context(), envelope(t, "restored", "host", 1, redirected))
	require.NoError(t, err)
	require.Equal(t, receipt.Conflicts, replay.Conflicts)
}
func TestSyncChecksSessionInsideTransactionAndRollsBack(t *testing.T) {
	s := testStore(t)
	target := synced(t, "first")
	_, err := s.Apply(t.Context(), envelope(t, "a", "host", 1, target))
	require.NoError(t, err)
	s.SessionCurrent = func(_ context.Context, tx *gorm.DB) error {
		// This sees the attempted update inside the still-uncommitted transaction.
		value, err := readSetting(tx, "backupTargetAuthority")
		require.NoError(t, err)
		require.Equal(t, "b", value)
		return errors.New("session replaced")
	}
	_, err = s.Apply(t.Context(), envelope(t, "b", "other", 1))
	require.EqualError(t, err, "session replaced")
	held, err := s.Resolve(t.Context(), target.ID, "host", true)
	require.NoError(t, err)
	require.Equal(t, target.ID, held.ID)
	value, err := readSetting(s.DB.DB, "backupTargetAuthority")
	require.NoError(t, err)
	require.Equal(t, "a", value)
}
func TestTargetsEncryptAndDoNotExposeSecrets(t *testing.T) {
	s := testStore(t)
	target := synced(t, "first")
	_, err := s.Apply(t.Context(), envelope(t, "a", "host", 1, target))
	require.NoError(t, err)
	var row Target
	require.NoError(t, s.DB.First(&row, "id = ?", target.ID).Error)
	require.NotContains(t, row.SecretEnc, target.Secret)
	require.NotContains(t, row.RecoveryKeyEnc, target.RecoveryKey)
	value, err := crypto.Decrypt(row.SecretEnc)
	require.NoError(t, err)
	require.Equal(t, target.Secret, value)
	projected := project(row, []Check{{ConfigVersion: 0}})
	bytes, err := json.Marshal(projected)
	require.NoError(t, err)
	require.NotContains(t, string(bytes), target.Secret)
	require.NotContains(t, string(bytes), target.RecoveryKey)
	require.True(t, projected.Checks[0].EarlierSettings)
	require.False(t, project(row, []Check{{ConfigVersion: 1}}).Checks[0].EarlierSettings)
	// A second host encrypts the dedicated sync DTO with its own key.
	originalCipher := row.SecretEnc
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "second-synthetic-encryption-key-32-bytes"})
	agent := testStore(t)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "second-synthetic-encryption-key-32-bytes"})
	_, err = agent.Apply(t.Context(), envelope(t, "a", "host", 1, target))
	require.NoError(t, err)
	require.NoError(t, agent.DB.First(&row, "id = ?", target.ID).Error)
	require.NotEqual(t, originalCipher, row.SecretEnc)
	value, err = crypto.Decrypt(row.SecretEnc)
	require.NoError(t, err)
	require.Equal(t, target.Secret, value)
	_, err = crypto.Decrypt(originalCipher)
	require.Error(t, err)
}
func TestHubTargetImmutabilityAndRecoveryKeyRoundTrip(t *testing.T) {
	store := testStore(t)
	s := NewService(Dependencies{Store: store})
	input := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "target"}
	target, err := s.Create(t.Context(), input)
	require.NoError(t, err)
	key, err := s.RecoveryKey(t.Context(), target.ID)
	require.NoError(t, err)
	require.NoError(t, backup.ValidateRecoveryKey(key))
	changed := input
	changed.Path = "/srv/other"
	_, err = s.Update(t.Context(), target.ID, changed)
	code(t, err, "backup.target.immutable")
	changed = input
	changed.Name = "renamed"
	updated, err := s.Update(t.Context(), target.ID, changed)
	require.NoError(t, err)
	require.Greater(t, updated.ConfigVersion, target.ConfigVersion)
	input.Name = "duplicate"
	_, err = s.Create(t.Context(), input)
	code(t, err, "backup.target.duplicate")
	input.Path = "/srv/attached"
	input.RecoveryKey = key
	attached, err := s.Create(t.Context(), input)
	require.NoError(t, err)
	exported, err := s.RecoveryKey(t.Context(), attached.ID)
	require.NoError(t, err)
	require.Equal(t, key, exported)
	authority, previousVersion, err := store.Authority(t.Context())
	require.NoError(t, err)
	require.NoError(t, store.ResetAuthority(t.Context()))
	next, version, err := store.Authority(t.Context())
	require.NoError(t, err)
	require.NotEqual(t, authority, next)
	require.EqualValues(t, previousVersion, version)
}
func TestRetirementFencesPolicyAndManualAdmission(t *testing.T) {
	store := testStore(t)
	atZero := make(chan struct{})
	commit := make(chan struct{})
	s := NewService(Dependencies{Store: store, References: func(context.Context, Target) (dto.TargetReferences, []string, error) {
		close(atZero)
		<-commit
		return dto.TargetReferences{}, nil, nil
	}})
	target, err := s.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "target"})
	require.NoError(t, err)
	retired := make(chan error, 1)
	go func() { retired <- s.Retire(t.Context(), target.ID) }()
	<-atZero
	results := make(chan error, 2)
	entered := make(chan struct{}, 2)
	for range 2 {
		go func() {
			entered <- struct{}{}
			results <- s.WithActive(t.Context(), target.ID, func() error { t.Error("admitted a new target reference during retirement"); return nil })
		}()
	}
	<-entered
	<-entered
	select {
	case err := <-results:
		t.Fatalf("admission passed retire fence: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(commit)
	require.NoError(t, <-retired)
	for range 2 {
		code(t, <-results, "backup.target.retired")
	}
	row, err := store.Resolve(t.Context(), target.ID, "0", false)
	require.NoError(t, err)
	require.Equal(t, target.ID, row.ID)
}
func TestRetireRefusesUnknownHostsAndReferences(t *testing.T) {
	for _, unknown := range []bool{true, false} {
		t.Run(map[bool]string{true: "unknown", false: "references"}[unknown], func(t *testing.T) {
			store := testStore(t)
			s := NewService(Dependencies{Store: store, References: func(context.Context, Target) (dto.TargetReferences, []string, error) {
				if unknown {
					return dto.TargetReferences{}, []string{"offline"}, nil
				}
				return dto.TargetReferences{Backups: []string{"data"}}, nil, nil
			}})
			target, err := s.Create(t.Context(), dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "target"})
			require.NoError(t, err)
			err = s.Retire(t.Context(), target.ID)
			if unknown {
				code(t, err, "backup.target.hosts-unknown")
			} else {
				code(t, err, "backup.target.in-use")
			}
			_, err = store.Resolve(t.Context(), target.ID, "0", true)
			require.NoError(t, err)
		})
	}
}

func TestRestoredAuthorityPreservesConfigurationVersionOrdering(t *testing.T) {
	store := testStore(t)
	service := NewService(Dependencies{Store: store})
	input := dto.TargetInput{TargetLocation: dto.TargetLocation{Kind: "local", HostID: new("0"), Path: "/srv/backups"}, Name: "first"}
	first, err := service.Create(t.Context(), input)
	require.NoError(t, err)
	input.Name = "second"
	input.Path = "/srv/other"
	second, err := service.Create(t.Context(), input)
	require.NoError(t, err)
	// An archive may contain an arbitrarily advanced counter and matching checks.
	require.NoError(t, writeSetting(store.DB.DB, "backup_target_set_version", "50"))
	require.NoError(t, store.DB.Model(&Target{}).Where("id IN ?", []string{first.ID, second.ID}).Update("config_version", 50).Error)
	for _, id := range []string{first.ID, second.ID} {
		require.NoError(t, store.DB.Create(&Check{TargetID: id, HostID: "0", SetVersion: 50, ConfigVersion: 50, ActivityID: "check", At: time.Now()}).Error)
	}
	require.NoError(t, store.ResetAuthority(t.Context()))
	input = dto.TargetInput{TargetLocation: first.location(), Name: "edited"}
	edited, err := service.Update(t.Context(), first.ID, input)
	require.NoError(t, err)
	require.EqualValues(t, 51, edited.ConfigVersion)
	current, err := service.Get(t.Context(), first.ID)
	require.NoError(t, err)
	require.True(t, current.Checks[0].EarlierSettings)
	untouched, err := service.Get(t.Context(), second.ID)
	require.NoError(t, err)
	require.False(t, untouched.Checks[0].EarlierSettings)
}
