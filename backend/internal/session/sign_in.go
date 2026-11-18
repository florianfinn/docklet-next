// Source: docklet-next/issues/68
// Revision: 92e896e926aaaf0189c72dcec6e3b15a52ddbd5d
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original atomic mode transition and session invalidation; M29 stored sign-in policy and setup claim state.

package session

import (
	"context"
	"errors"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/types/auth"
	"gorm.io/gorm"
)

// Stored sign-in policy rows (settings table, internal keys) and the first-run claim marker.
const (
	SettingAuthMode           = "authMode"
	SettingAuthProxyHeader    = "authProxyHeader"
	SettingAuthTrustedProxies = "authTrustedProxies"
	SettingSetupIdentity      = "setupIdentity"
	SettingAuthLastMode       = "authLastMode"
	SetupIdentityDone         = "done"
)

// StoredSignInPolicy is the complete stored triple; an empty Mode means nothing was stored yet.
type StoredSignInPolicy struct {
	Mode           auth.SignInMode
	ProxyHeader    string
	TrustedProxies string
}

func (s *SessionService) ActivateSignInMode(ctx context.Context, mode auth.SignInMode) error {
	return s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		return ActivateSignInModeInDB(tx, mode, false)
	})
}

// ActivateSignInModeInDB revokes every open session when the mode differs from authLastMode,
// or always when revokeAlways is set (policy writes), and records the mode.
func ActivateSignInModeInDB(tx *gorm.DB, mode auth.SignInMode, revokeAlways bool) error {
	var previous settings.SettingVariable
	err := tx.Where("key = ?", SettingAuthLastMode).First(&previous).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if !revokeAlways && err == nil && previous.Value == string(mode) {
		return nil
	}
	now := time.Now().UTC()
	if err := tx.Model(&UserSession{}).Where("revoked_at IS NULL").Updates(map[string]any{"revoked_at": now, "updated_at": now}).Error; err != nil {
		return err
	}
	return tx.Save(&settings.SettingVariable{Key: SettingAuthLastMode, Value: string(mode)}).Error
}

// Transaction runs fn in one database write transaction (claim and policy writes span users, roles and settings).
func (s *SessionService) Transaction(ctx context.Context, fn func(tx *gorm.DB) error) error {
	return s.db.WithContext(ctx).Transaction(fn)
}

// LoadSignInState reads the stored policy and whether the first-run claim is done.
func (s *SessionService) LoadSignInState(ctx context.Context) (StoredSignInPolicy, bool, error) {
	var rows []settings.SettingVariable
	keys := []string{SettingAuthMode, SettingAuthProxyHeader, SettingAuthTrustedProxies, SettingSetupIdentity}
	if err := s.db.WithContext(ctx).Where("key IN ?", keys).Find(&rows).Error; err != nil {
		return StoredSignInPolicy{}, false, err
	}
	var stored StoredSignInPolicy
	done := false
	for _, row := range rows {
		switch row.Key {
		case SettingAuthMode:
			stored.Mode = auth.SignInMode(row.Value)
		case SettingAuthProxyHeader:
			stored.ProxyHeader = row.Value
		case SettingAuthTrustedProxies:
			stored.TrustedProxies = row.Value
		case SettingSetupIdentity:
			done = row.Value == SetupIdentityDone
		}
	}
	return stored, done, nil
}

// WriteSignInPolicyInDB stores the complete triple.
func WriteSignInPolicyInDB(tx *gorm.DB, policy StoredSignInPolicy) error {
	for key, value := range map[string]string{
		SettingAuthMode:           string(policy.Mode),
		SettingAuthProxyHeader:    policy.ProxyHeader,
		SettingAuthTrustedProxies: policy.TrustedProxies,
	} {
		if err := tx.Save(&settings.SettingVariable{Key: key, Value: value}).Error; err != nil {
			return err
		}
	}
	return nil
}
