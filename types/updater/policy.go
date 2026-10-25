// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Hub-owned policy, schedule and durable notification projections.

package updater

import (
	"github.com/florianfinn/docklet-next/types/jobschedule"
	"time"
)

type PolicyBinding struct {
	PolicyID string `json:"policyId"`
	Revision uint64 `json:"revision"`
	JobRunID string `json:"jobRunId"`
	ChildID  string `json:"childId"`
}
type AutoUpdatePolicy struct {
	Mode           string            `json:"mode" enum:"off,notify,apply"`
	Revision       uint64            `json:"revision"`
	EnvironmentID  string            `json:"environmentId"`
	RegistrationID string            `json:"registrationId"`
	TargetKind     string            `json:"targetKind" enum:"project"`
	TargetID       string            `json:"targetId"`
	TargetIdentity string            `json:"targetIdentity"`
	JobID          string            `json:"jobId"`
	CheckedAt      *time.Time        `json:"checkedAt,omitempty"`
	Stale          bool              `json:"stale"`
	Code           string            `json:"code,omitempty"`
	Candidates     []UpdateCandidate `json:"candidates"`
}
type UpdatePolicyInput struct {
	Mode             string `json:"mode" enum:"off,notify,apply"`
	ExpectedRevision uint64 `json:"expectedRevision"`
}
type UpdateCandidate struct {
	Service           string `json:"service"`
	ImageRef          string `json:"imageRef"`
	Version           string `json:"version"`
	Digest            string `json:"digest"`
	Platform          string `json:"platform"`
	EntryID           string `json:"entryId,omitempty"`
	NotificationState string `json:"notificationState" enum:"pending,sent,unconfigured,failed,not-requested"`
}
type UpdateScheduleInput struct {
	ExpectedRevision   uint64 `json:"expectedRevision"`
	PollingInterval    string `json:"pollingInterval" minLength:"1" maxLength:"128"`
	AutoUpdateInterval string `json:"autoUpdateInterval" minLength:"1" maxLength:"128"`
	PollingEnabled     bool   `json:"pollingEnabled"`
	AutoUpdateEnabled  bool   `json:"autoUpdateEnabled"`
}
type UpdateScheduleConfig struct {
	Revision                uint64                  `json:"revision"`
	EnvironmentID           string                  `json:"environmentId"`
	RegistrationID          string                  `json:"registrationId"`
	PollingInterval         string                  `json:"pollingInterval"`
	AutoUpdateInterval      string                  `json:"autoUpdateInterval"`
	PollingEnabled          bool                    `json:"pollingEnabled"`
	AutoUpdateEnabled       bool                    `json:"autoUpdateEnabled"`
	MasterPollingEnabled    bool                    `json:"masterPollingEnabled"`
	MasterAutoUpdateEnabled bool                    `json:"masterAutoUpdateEnabled"`
	TimeZone                string                  `json:"timeZone"`
	Jobs                    []jobschedule.JobStatus `json:"jobs"`
}
