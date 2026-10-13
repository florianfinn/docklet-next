// Source: getarcaneapp/arcane/backend/internal/backup/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package backup

import "github.com/florianfinn/docklet-next/backend/internal/database"

// SystemBackupRecoveryConfig stores the encrypted recovery key; it must be reversible because Rustic needs the plaintext.
type SystemBackupRecoveryConfig struct {
	database.BaseModel

	EncryptedRecoveryKey string `gorm:"column:encrypted_recovery_key;type:text;not null"`
}

func (SystemBackupRecoveryConfig) TableName() string { return "system_backup_recovery_config" }
