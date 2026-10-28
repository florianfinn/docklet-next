// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original volume backup lifecycle contract.

package backup

import "time"

// TargetLocation is immutable: one target ID always names the same repository.
type TargetLocation struct {
	Kind   string  `json:"kind" enum:"local,mount,smb,nfs"`
	HostID *string `json:"hostId,omitempty"`
	Path   string  `json:"path"`
	Server string  `json:"server,omitempty"`
	Share  string  `json:"share,omitempty"`
}

type Target struct {
	TargetLocation
	ID            string        `json:"id"`
	Name          string        `json:"name"`
	Username      string        `json:"username,omitempty"`
	Options       string        `json:"options,omitempty"`
	HasSecret     bool          `json:"hasSecret"`
	ConfigVersion int64         `json:"configVersion"`
	CreatedAt     time.Time     `json:"createdAt"`
	UpdatedAt     *time.Time    `json:"updatedAt,omitempty"`
	RetiredAt     *time.Time    `json:"retiredAt,omitempty"`
	Checks        []TargetCheck `json:"checks"`
}

type TargetInput struct {
	TargetLocation
	Name        string  `json:"name" minLength:"1" maxLength:"64"`
	Username    string  `json:"username,omitempty"`
	Secret      *string `json:"secret,omitempty" writeOnly:"true"`
	Options     string  `json:"options,omitempty"`
	RecoveryKey string  `json:"recoveryKey,omitempty" writeOnly:"true"`
}

// SyncedTarget is sent only on the authenticated hub-to-agent metadata channel.
type SyncedTarget struct {
	TargetLocation
	ID            string `json:"id"`
	Name          string `json:"name"`
	Username      string `json:"username,omitempty"`
	Secret        string `json:"secret,omitempty"`
	Options       string `json:"options,omitempty"`
	RecoveryKey   string `json:"recoveryKey"`
	ConfigVersion int64  `json:"configVersion"`
}

type TargetSet struct {
	Authority   string         `json:"authority"`
	SetVersion  int64          `json:"setVersion" minimum:"1"`
	HubHostID   string         `json:"hubHostId"`
	ContentHash string         `json:"contentHash"`
	Targets     []SyncedTarget `json:"targets"`
}

type TargetSetReceipt struct {
	Authority   string   `json:"authority"`
	SetVersion  int64    `json:"setVersion"`
	ContentHash string   `json:"contentHash"`
	Conflicts   []string `json:"conflicts"`
}

type TargetCheck struct {
	HostID          string    `json:"hostId"`
	SetVersion      int64     `json:"setVersion"`
	ConfigVersion   int64     `json:"configVersion"`
	ActivityID      string    `json:"activityId"`
	OK              bool      `json:"ok"`
	Code            string    `json:"code,omitempty"`
	FreeBytes       *int64    `json:"freeBytes,omitempty" minimum:"0"`
	TotalBytes      *int64    `json:"totalBytes,omitempty" minimum:"0"`
	SnapshotCount   *int64    `json:"snapshotCount,omitempty" minimum:"0"`
	At              time.Time `json:"at"`
	EarlierSettings bool      `json:"earlierSettings"`
}

type TargetReferences struct {
	Policies []string `json:"policies"`
	Backups  []string `json:"backups"`
	Running  []string `json:"running"`
}

type TargetTestRequest struct {
	HostID string `json:"hostId"`
}
type TargetTestOperation struct {
	ActivityID string `json:"activityId"`
}
type TargetRecoveryKey struct {
	RecoveryKey string `json:"recoveryKey"`
}
