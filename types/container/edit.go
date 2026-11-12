// Source: getarcaneapp/arcane/types/container/container.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; Edit moved unchanged from types/container/container.go.

package container

import "github.com/florianfinn/docklet-next/types/containerregistry"

// Edit describes configuration changes to apply when recreating a container.
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
