// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package system

const (
	SelfUpdateNotCompose        = "update.self-not-compose"
	SelfUpdateUnsupportedLayout = "update.self-unsupported-layout"
	SelfUpdateCustomImage       = "update.self-custom-image"
	SelfUpdateRequiresHub       = "update.requires-hub-update"
	SelfUpdateFailed            = "update.self-failed"
	SelfUpdateOutcomeUnknown    = "update.outcome-unknown"
)

// SelfUpdateEnvelope is the frozen, authenticated hub-to-agent recovery request.
// Browser input is never authoritative for its target or hub version.
type SelfUpdateEnvelope struct {
	OpID            string `json:"opId,omitempty" minLength:"1" maxLength:"64"`
	TargetVersion   string `json:"targetVersion,omitempty"`
	TargetReference string `json:"targetReference,omitempty"`
	HubVersion      string `json:"hubVersion,omitempty"`
}

type SelfUpdateResponse struct {
	OpID     string `json:"opId"`
	Accepted bool   `json:"accepted"`
	Phase    string `json:"phase" enum:"reserved,helper-started,settled"`
	Code     string `json:"code,omitempty" enum:"feature-disabled,update.check-unavailable,host.busy,update.self-not-compose,update.self-unsupported-layout,update.self-custom-image,update.requires-hub-update,update.self-failed,update.outcome-unknown,update.invalid-target,update.system-version-unknown,agent.unavailable"`
	RunID    string `json:"runId,omitempty"`
}

type SelfUpdateOperation struct {
	OpID          string `json:"opId"`
	Phase         string `json:"phase" enum:"reserved,helper-started,settled"`
	Outcome       string `json:"outcome,omitempty" enum:"healthy,failed-health,failed-pre-effect,unknown"`
	Code          string `json:"code,omitempty"`
	TargetVersion string `json:"targetVersion"`
	RunID         string `json:"runId,omitempty"`
	HelperRunning bool   `json:"helperRunning"`
}

type SelfUpdateCandidate struct {
	Version   string `json:"version"`
	Reference string `json:"reference,omitempty"`
}

type SelfUpdateEligibility struct {
	State     string               `json:"state" enum:"available,current,checking,not-eligible,unavailable"`
	Candidate *SelfUpdateCandidate `json:"candidate,omitempty"`
	Code      string               `json:"code,omitempty" enum:"feature-disabled,update.check-unavailable,host.busy,update.self-not-compose,update.self-unsupported-layout,update.self-custom-image,update.requires-hub-update,update.self-failed,update.outcome-unknown,update.invalid-target,update.system-version-unknown,agent.unavailable"`
	Manual    []string             `json:"manual"`
}

type SelfUpdateCheck struct {
	Version     string                `json:"version"`
	Eligibility SelfUpdateEligibility `json:"eligibility"`
	Operation   *SelfUpdateOperation  `json:"operation,omitempty"`
}
