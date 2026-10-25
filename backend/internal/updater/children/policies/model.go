// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Hub registration-bound policies, child checkpoints and notification outbox.

package policies

import "time"

type Policy struct {
	ID             string `gorm:"primaryKey;not null"`
	EnvironmentID  string `gorm:"not null;index"`
	RegistrationID string `gorm:"not null"`
	TargetID       string `gorm:"not null"`
	TargetIdentity string `gorm:"not null"`
	Mode           string `gorm:"not null;check:chk_stack_policy_mode,mode IN ('off','notify','apply')"`
	Revision       uint64 `gorm:"not null;check:revision > 0"`
	ActorID        string `gorm:"not null"`
	ActorKeyID     string `gorm:"not null"`
	CheckedAt      *time.Time
	Code           string `gorm:"not null"`
	Candidates     string `gorm:"not null"`
}

func (Policy) TableName() string { return "stack_update_policies" }

type HostSchedule struct {
	ID                 string `gorm:"primaryKey;not null"`
	EnvironmentID      string `gorm:"not null;uniqueIndex"`
	RegistrationID     string `gorm:"not null"`
	Revision           uint64 `gorm:"not null;check:revision > 0"`
	PollingInterval    string `gorm:"not null"`
	AutoUpdateInterval string `gorm:"not null"`
	PollingEnabled     bool   `gorm:"not null;check:polling_enabled IN (0,1)"`
	AutoUpdateEnabled  bool   `gorm:"not null;check:auto_update_enabled IN (0,1)"`
	ActorID            string `gorm:"not null"`
	ActorKeyID         string `gorm:"not null"`
}

func (HostSchedule) TableName() string { return "image_update_schedules" }

// Child is committed before preflight/admission. Runtime admission changes it to
// admitted in the receipt transaction; restart can therefore only query, not resend.
type Child struct {
	ID        string `gorm:"primaryKey;not null"`
	JobRunID  string `gorm:"not null;uniqueIndex:idx_update_child_target"`
	PolicyID  string `gorm:"not null;uniqueIndex:idx_update_child_target"`
	Revision  uint64 `gorm:"not null;check:revision > 0"`
	Status    string `gorm:"not null;check:status IN ('prepared','admitted','refused','settled')"`
	Code      string `gorm:"not null"`
	RuntimeID string `gorm:"not null"`
}

func (Child) TableName() string { return "image_policy_children" }

type Candidate struct {
	ID        string    `gorm:"primaryKey;not null"`
	PolicyID  string    `gorm:"not null;index"`
	CreatedAt time.Time `gorm:"not null"`
}

func (Candidate) TableName() string { return "image_update_candidates" }

type Notice struct {
	ID            string    `gorm:"primaryKey;not null"`
	CandidateID   string    `gorm:"not null;uniqueIndex"`
	EnvironmentID string    `gorm:"not null"`
	Payload       string    `gorm:"not null"`
	Status        string    `gorm:"not null;check:status IN ('pending','sending','sent','unconfigured','failed')"`
	Delivered     int       `gorm:"not null;check:delivered >= 0"`
	CreatedAt     time.Time `gorm:"not null"`
}

func (Notice) TableName() string { return "image_update_notifications" }
