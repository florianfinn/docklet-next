// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original transaction-local read of the agent pairing credential.

package environment

import (
	"crypto/subtle"
	"errors"

	"gorm.io/gorm"
)

// AgentPairingTokenCurrent reads through the caller's transaction, so a sync
// cannot commit against a token superseded by an already-committed pairing.
// fallback is the startup-configured token while no pairing has stored one;
// default settings seed an empty row, which counts as "none stored".
func AgentPairingTokenCurrent(tx *gorm.DB, presented, fallback string) error {
	var row struct{ Value string }
	err := tx.Table("settings").Where("key = ?", "agentToken").Take(&row).Error
	if err != nil && !errors.Is(err, gorm.ErrRecordNotFound) {
		return err
	}
	if row.Value == "" {
		row.Value = fallback
	}
	if presented == "" || row.Value == "" || subtle.ConstantTimeCompare([]byte(presented), []byte(row.Value)) != 1 {
		return ErrInvalidEnvironmentAccessToken
	}
	return nil
}
