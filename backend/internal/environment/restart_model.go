// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package environment

import "time"

// AgentRestartRequest persists the one-shot request and the observed process binding.
type AgentRestartRequest struct {
	RunID              string    `gorm:"primaryKey;not null"`
	EnvironmentID      string    `gorm:"not null"`
	RegistrationID     string    `gorm:"not null"`
	CredentialRevision int64     `gorm:"not null"`
	OldInstance        string    `gorm:"not null"`
	State              string    `gorm:"not null"`
	CreatedAt          time.Time `gorm:"not null"`
	UpdatedAt          time.Time `gorm:"not null"`
	SentAt             *time.Time
	AcknowledgedAt     *time.Time
	Code               *string
	Deadline           time.Time `gorm:"not null"`
	SettledAt          *time.Time
}

func (AgentRestartRequest) TableName() string { return "agent_restart_requests" }
