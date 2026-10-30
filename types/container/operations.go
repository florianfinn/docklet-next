// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package container

// Origin is determined on the reading host and overlaid by the hub assignment.
type Origin string
type OriginSource string

const (
	OriginCompose     Origin       = "compose"
	OriginUnraid      Origin       = "unraid"
	OriginUnknown     Origin       = "unknown"
	OriginStandalone  Origin       = "standalone"
	OriginDetected    OriginSource = "detected"
	OriginManual      OriginSource = "manual"
	OriginUnconfirmed OriginSource = "unconfirmed"
)

type OriginFields struct {
	Origin       Origin       `json:"origin" enum:"compose,unraid,unknown,standalone"`
	OriginSource OriginSource `json:"originSource" enum:"detected,manual,unconfirmed"`
}
type OriginAssignment struct {
	Origin   Origin `json:"origin" enum:"unraid,standalone"`
	DockerID string `json:"dockerId" minLength:"1" maxLength:"256"`
}
type RecreateRequest struct {
	Edit Edit `json:"edit"`
}
type RecreateResult struct {
	OldID   string `json:"oldId"`
	NewID   string `json:"newId"`
	OldName string `json:"oldName"`
	NewName string `json:"newName"`
}

// Problem exposes stable codes without Docker or submitted configuration text.
type Problem struct {
	Code   string `json:"code" enum:"container.origin-locked,container.recreate-recovered,container.recreate-ambiguous,runtime.idempotency-mismatch,update.path-retired,container.shell-invalid,container.shell-protocol,container.shell-unavailable,container.shell-orphaned,container.shell-not-running"`
	Origin Origin `json:"origin,omitempty" enum:"compose,unraid,unknown,standalone"`
	Status int    `json:"status"`
	step   string
}

func (p *Problem) Error() string  { return p.Code }
func (p *Problem) GetStatus() int { return p.Status }

// RecreateFailure carries the failed recreate step as bounded evidence; it never
// includes Docker error text, which may echo edit values.
func RecreateFailure(code, step string) *Problem {
	return &Problem{Code: code, Status: 409, step: step}
}

// Step names the recreate effect that failed, or "" when unknown.
func (p *Problem) Step() string { return p.step }

// ShellQuery is validated by the hub before any local or remote upgrade.
type ShellQuery struct {
	Shell string `json:"shell" enum:"auto,sh,bash" default:"auto"`
	Cols  *int   `json:"cols,omitempty" minimum:"1" maximum:"1000"`
	Rows  *int   `json:"rows,omitempty" minimum:"1" maximum:"1000"`
}

const ShellFrameProtocolVersion = 1

type ShellResize struct {
	Type string `json:"type" enum:"resize"`
	Cols int    `json:"cols" minimum:"1" maximum:"1000"`
	Rows int    `json:"rows" minimum:"1" maximum:"1000"`
}
type ShellExit struct {
	Type   string `json:"type" enum:"exit"`
	Code   *int   `json:"code"`
	Reason string `json:"reason"`
}
