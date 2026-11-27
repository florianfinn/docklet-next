// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original hub-only combined update contract.

package system

// Codes of the combined update and of its skipped hosts.
const (
	FleetBusy          = "update.all-busy"
	FleetCurrent       = "update.current"
	FleetHubFailed     = "update.hub-failed"
	FleetNewerThanHub  = "update.newer-than-target"
	FleetUnreadable    = "update.fleet-unreadable"
	FleetPermission    = "update.permission-denied"
	FleetTargetChanged = "update.target-changed"
	FleetAgentTooOld   = "agent.update-required"
)

// FleetUpdateRequest confirms the version shown to the administrator.
type FleetUpdateRequest struct {
	TargetVersion string `json:"targetVersion" minLength:"1" maxLength:"64"`
}

type FleetUpdateResponse struct {
	FleetID  string `json:"fleetId"`
	RunID    string `json:"runId"`
	HubOpID  string `json:"hubOpId,omitempty"`
	HubPhase string `json:"hubPhase" enum:"update,current"`
}

type FleetUpdateTarget struct {
	EnvironmentID string `json:"environmentId"`
	State         string `json:"state" enum:"pending,reserved,skipped,settled"`
	Outcome       string `json:"outcome,omitempty" enum:"healthy,current,failed-health,failed-pre-effect,unknown,skipped"`
	Code          string `json:"code,omitempty" enum:"update.all-busy,update.current,update.hub-failed,update.newer-than-target,update.fleet-unreadable,update.permission-denied,update.target-changed,update.invalid-target,update.check-unavailable,update.system-version-unknown,update.self-not-compose,update.self-unsupported-layout,update.self-custom-image,update.self-failed,update.outcome-unknown,update.requires-hub-update,feature-disabled,host.busy,agent.unavailable,agent.update-required"`
	RunID         string `json:"runId,omitempty"`
	OpID          string `json:"opId"`
}

// FleetUpdateStatus is the active or last combined update; Active is false
// when no plan was ever stored.
type FleetUpdateStatus struct {
	Active        bool                `json:"active"`
	FleetID       string              `json:"fleetId,omitempty"`
	RunID         string              `json:"runId,omitempty"`
	Phase         string              `json:"phase,omitempty" enum:"hub,agents,settled"`
	Outcome       string              `json:"outcome,omitempty" enum:"success,partial,failed"`
	TargetVersion string              `json:"targetVersion,omitempty"`
	Hub           *FleetUpdateTarget  `json:"hub,omitempty"`
	Agents        []FleetUpdateTarget `json:"agents"`
}
