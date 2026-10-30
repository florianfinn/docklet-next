// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M14 container and host operations foundation.

package container

import "github.com/florianfinn/docklet-next/types/containerregistry"

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
}

func (p *Problem) Error() string  { return p.Code }
func (p *Problem) GetStatus() int { return p.Status }

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

// Each form-owned section is a pointer: nil preserves the inspected value,
// non-nil (even empty) replaces that section. Settings not represented here
// (sysctls, ulimits, log drivers, devices, tmpfs, DNS, ...) always carry over
// from the existing container untouched.
type Edit struct {
	// Name is the new container name.
	//
	// Required: false
	Name *string `json:"name,omitempty"`

	// Image reference to run.
	//
	// Required: false
	Image *string `json:"image,omitempty"`

	// WorkingDir is the working directory inside the container.
	//
	// Required: false
	WorkingDir *string `json:"workingDir,omitempty"`

	// User to run the container as.
	//
	// Required: false
	User *string `json:"user,omitempty"`

	// Command to run in the container.
	//
	// Required: false
	Command *[]string `json:"command,omitempty"`

	// Entrypoint for the container.
	//
	// Required: false
	Entrypoint *[]string `json:"entrypoint,omitempty"`

	// Environment variables for the container.
	//
	// Required: false
	Environment *[]string `json:"environment,omitempty"`

	// Labels to set on the container.
	//
	// Required: false
	Labels *map[string]string `json:"labels,omitempty"`

	// Healthcheck replaces the container healthcheck when set.
	//
	// Required: false
	Healthcheck *HealthcheckCreate `json:"healthcheck,omitempty"`

	// ClearHealthcheck reverts to the image-defined healthcheck.
	//
	// Required: false
	ClearHealthcheck bool `json:"clearHealthcheck,omitempty"`

	// HostConfig holds host-level configuration changes.
	//
	// Required: false
	HostConfig *HostConfigEdit `json:"hostConfig,omitempty"`

	// NetworkingConfig is the full desired network endpoint set. Nil preserves
	// the existing attachments.
	//
	// Required: false
	NetworkingConfig *NetworkingConfigCreate `json:"networkingConfig,omitempty"`

	// Credentials for pulling images from private registries.
	//
	// Required: false
	Credentials []containerregistry.Credential `json:"credentials,omitempty"`
}
