// Source: getarcaneapp/arcane/types/container/container.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; M14 container and host operation contracts; split for #265, manifest descriptor platform with explicit unknown; M18 live scheduling, policy and contract foundation.

/*
BSD 3-Clause License

Copyright (c) 2025, Kyle Mendell

Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice, this
   list of conditions and the following disclaimer.

2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

3. Neither the name of the copyright holder nor the names of its
   contributors may be used to endorse or promote products derived from
   this software without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE ARE
DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE LIABLE
FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR CONSEQUENTIAL
DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF SUBSTITUTE GOODS OR
SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS INTERRUPTION) HOWEVER
CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN CONTRACT, STRICT LIABILITY,
OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE) ARISING IN ANY WAY OUT OF THE USE
OF THIS SOFTWARE, EVEN IF ADVISED OF THE POSSIBILITY OF SUCH DAMAGE.
*/

package container

import (
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
)

// HostConfigEdit carries host-level configuration changes for a container
// edit. Each field is a pointer: nil preserves the existing value, non-nil
// (even empty) replaces it.
type HostConfigEdit struct {
	// Binds is the full desired list of volume bindings.
	//
	// Required: false
	Binds *[]string `json:"binds,omitempty"`

	// Mounts is the full desired list of typed mounts.
	//
	// Required: false
	Mounts *[]MountCreate `json:"mounts,omitempty"`

	// PortBindings is the full desired port binding map.
	//
	// Required: false
	PortBindings *map[string][]PortBindingCreate `json:"portBindings,omitempty"`

	// RestartPolicy for the container.
	//
	// Required: false
	RestartPolicy *RestartPolicyCreate `json:"restartPolicy,omitempty"`

	// Privileged indicates if the container runs in privileged mode.
	//
	// Required: false
	Privileged *bool `json:"privileged,omitempty"`

	// CapAdd is the full desired list of added kernel capabilities.
	//
	// Required: false
	CapAdd *[]string `json:"capAdd,omitempty"`

	// CapDrop is the full desired list of dropped kernel capabilities.
	//
	// Required: false
	CapDrop *[]string `json:"capDrop,omitempty"`

	// AutoRemove indicates if the container is removed when stopped.
	//
	// Required: false
	AutoRemove *bool `json:"autoRemove,omitempty"`

	// ReadonlyRootfs makes the root filesystem read-only.
	//
	// Required: false
	ReadonlyRootfs *bool `json:"readonlyRootfs,omitempty"`

	// Memory limit in bytes.
	//
	// Required: false
	Memory *int64 `json:"memory,omitempty"`

	// MemorySwap limits total memory usage (memory + swap) in bytes.
	//
	// Required: false
	MemorySwap *int64 `json:"memorySwap,omitempty"`

	// NanoCPUs is CPU allocation in nano CPUs.
	//
	// Required: false
	NanoCPUs *int64 `json:"nanoCpus,omitempty"`

	// CPUShares is the relative CPU share weight.
	//
	// Required: false
	CPUShares *int64 `json:"cpuShares,omitempty"`
}

// EditConfigNetwork describes one desired network attachment as shown in the
// edit form.
type EditConfigNetwork struct {
	// Aliases for the container on this network.
	//
	// Required: false
	Aliases []string `json:"aliases,omitempty"`

	// IPv4Address is the configured static IPv4 address, if any.
	//
	// Required: false
	IPv4Address string `json:"ipv4Address,omitempty"`

	// IPv6Address is the configured static IPv6 address, if any.
	//
	// Required: false
	IPv6Address string `json:"ipv6Address,omitempty"`
}

// EditConfigHostConfig is the host-config subset surfaced by the edit form.
type EditConfigHostConfig struct {
	// Binds is the list of volume bindings.
	//
	// Required: false
	Binds []string `json:"binds,omitempty"`

	// Mounts is the list of typed mounts.
	//
	// Required: false
	Mounts []MountCreate `json:"mounts,omitempty"`

	// PortBindings maps container ports to host bindings.
	//
	// Required: false
	PortBindings map[string][]PortBindingCreate `json:"portBindings,omitempty"`

	// RestartPolicy for the container.
	//
	// Required: false
	RestartPolicy RestartPolicyCreate `json:"restartPolicy"`

	// NetworkMode for the container.
	//
	// Required: false
	NetworkMode string `json:"networkMode,omitempty"`

	// Privileged indicates if the container runs in privileged mode.
	//
	// Required: false
	Privileged bool `json:"privileged,omitempty"`

	// CapAdd is the list of added kernel capabilities.
	//
	// Required: false
	CapAdd []string `json:"capAdd,omitempty"`

	// CapDrop is the list of dropped kernel capabilities.
	//
	// Required: false
	CapDrop []string `json:"capDrop,omitempty"`

	// AutoRemove indicates if the container is removed when stopped.
	//
	// Required: false
	AutoRemove bool `json:"autoRemove,omitempty"`

	// ReadonlyRootfs indicates a read-only root filesystem.
	//
	// Required: false
	ReadonlyRootfs bool `json:"readonlyRootfs,omitempty"`

	// Memory limit in bytes.
	//
	// Required: false
	Memory int64 `json:"memory,omitempty"`

	// MemorySwap limits total memory usage (memory + swap) in bytes.
	//
	// Required: false
	MemorySwap int64 `json:"memorySwap,omitempty"`

	// NanoCPUs is CPU allocation in nano CPUs.
	//
	// Required: false
	NanoCPUs int64 `json:"nanoCpus,omitempty"`

	// CPUShares is the relative CPU share weight.
	//
	// Required: false
	CPUShares int64 `json:"cpuShares,omitempty"`
}

// EditConfig is the read DTO backing the container edit form.
type EditConfig struct {
	// ID is the unique identifier of the container.
	//
	// Required: true
	ID string `json:"id"`

	// Name of the container.
	//
	// Required: true
	Name string `json:"name"`

	// Image reference the container was created from.
	//
	// Required: true
	Image string `json:"image"`

	// Command the container runs.
	//
	// Required: false
	Command []string `json:"command,omitempty"`

	// Entrypoint for the container.
	//
	// Required: false
	Entrypoint []string `json:"entrypoint,omitempty"`

	// WorkingDir is the working directory inside the container.
	//
	// Required: false
	WorkingDir string `json:"workingDir,omitempty"`

	// User the container runs as.
	//
	// Required: false
	User string `json:"user,omitempty"`

	// Environment variables of the container.
	//
	// Required: false
	Environment []string `json:"environment,omitempty"`

	// Labels set on the container.
	//
	// Required: false
	Labels map[string]string `json:"labels,omitempty"`

	// Healthcheck configuration (durations in seconds), if the container
	// defines one.
	//
	// Required: false
	Healthcheck *HealthcheckCreate `json:"healthcheck,omitempty"`

	// HostConfig is the host-config subset surfaced by the edit form.
	//
	// Required: true
	HostConfig EditConfigHostConfig `json:"hostConfig"`

	// Networks maps network names to their desired endpoint settings.
	//
	// Required: false
	Networks map[string]EditConfigNetwork `json:"networks,omitempty"`

	// IsCompose indicates the container is managed by a compose project.
	//
	// Required: false
	IsCompose bool `json:"isCompose,omitempty"`

	// ComposeProject is the owning compose project name, if any.
	//
	// Required: false
	ComposeProject string `json:"composeProject,omitempty"`

	// EditDisabled indicates edit actions are disabled for this container.
	//
	// Required: false
	EditDisabled bool `json:"editDisabled,omitempty"`

	// Running indicates whether the container is currently running.
	//
	// Required: true
	Running bool `json:"running"`
}

// NewEditConfigFromInspect builds an EditConfig from a docker inspect response.
func NewEditConfigFromInspect(c *container.InspectResponse, isCompose, editDisabled bool) EditConfig {
	cfg := EditConfig{
		ID:           c.ID,
		Name:         strings.TrimPrefix(c.Name, "/"),
		IsCompose:    isCompose,
		EditDisabled: editDisabled,
		Running:      c.State != nil && c.State.Running,
	}

	if c.Config != nil {
		cfg.Image = c.Config.Image
		cfg.Command = append([]string{}, c.Config.Cmd...)
		cfg.Entrypoint = append([]string{}, c.Config.Entrypoint...)
		cfg.WorkingDir = c.Config.WorkingDir
		cfg.User = c.Config.User
		cfg.Environment = append([]string{}, c.Config.Env...)
		cfg.Labels = c.Config.Labels
		cfg.ComposeProject = c.Config.Labels["com.docker.compose.project"]
		cfg.Healthcheck = mapEditHealthcheck(c.Config.Healthcheck)
	}

	cfg.HostConfig = mapEditHostConfig(c.HostConfig)
	cfg.Networks = mapEditNetworks(c)

	return cfg
}

func mapEditHealthcheck(hc *container.HealthConfig) *HealthcheckCreate {
	if hc == nil {
		return nil
	}

	return &HealthcheckCreate{
		Test:          append([]string{}, hc.Test...),
		Interval:      int64(hc.Interval / time.Second),
		Timeout:       int64(hc.Timeout / time.Second),
		StartPeriod:   int64(hc.StartPeriod / time.Second),
		StartInterval: int64(hc.StartInterval / time.Second),
		Retries:       hc.Retries,
	}
}

func mapEditHostConfig(hc *container.HostConfig) EditConfigHostConfig {
	if hc == nil {
		return EditConfigHostConfig{}
	}

	out := EditConfigHostConfig{
		Binds: append([]string{}, hc.Binds...),
		RestartPolicy: RestartPolicyCreate{
			Name:              string(hc.RestartPolicy.Name),
			MaximumRetryCount: hc.RestartPolicy.MaximumRetryCount,
		},
		NetworkMode:    string(hc.NetworkMode),
		Privileged:     hc.Privileged,
		CapAdd:         append([]string{}, hc.CapAdd...),
		CapDrop:        append([]string{}, hc.CapDrop...),
		AutoRemove:     hc.AutoRemove,
		ReadonlyRootfs: hc.ReadonlyRootfs,
		Memory:         hc.Memory,
		MemorySwap:     hc.MemorySwap,
		NanoCPUs:       hc.NanoCPUs,
		CPUShares:      hc.CPUShares,
	}

	for _, m := range hc.Mounts {
		out.Mounts = append(out.Mounts, MountCreate{
			Type:     string(m.Type),
			Source:   m.Source,
			Target:   m.Target,
			ReadOnly: m.ReadOnly,
		})
	}

	if len(hc.PortBindings) > 0 {
		out.PortBindings = make(map[string][]PortBindingCreate, len(hc.PortBindings))
		for port, bindings := range hc.PortBindings {
			mapped := make([]PortBindingCreate, 0, len(bindings))
			for _, b := range bindings {
				hostIP := ""
				if b.HostIP.IsValid() {
					hostIP = b.HostIP.String()
				}
				mapped = append(mapped, PortBindingCreate{
					HostIP:   hostIP,
					HostPort: b.HostPort,
				})
			}
			out.PortBindings[port.String()] = mapped
		}
	}

	return out
}

func mapEditNetworks(c *container.InspectResponse) map[string]EditConfigNetwork {
	if c.NetworkSettings == nil || len(c.NetworkSettings.Networks) == 0 {
		return nil
	}

	shortID := ""
	if len(c.ID) >= 12 {
		shortID = c.ID[:12]
	}

	out := make(map[string]EditConfigNetwork, len(c.NetworkSettings.Networks))
	for name, endpoint := range c.NetworkSettings.Networks {
		mapped := EditConfigNetwork{}
		if endpoint != nil {
			// The daemon injects the container short ID as an implicit alias;
			// it is regenerated on recreate and must not round-trip as a
			// user-defined alias.
			for _, alias := range endpoint.Aliases {
				if alias == "" || alias == shortID {
					continue
				}
				mapped.Aliases = append(mapped.Aliases, alias)
			}
			if endpoint.IPAMConfig != nil {
				if endpoint.IPAMConfig.IPv4Address.IsValid() {
					mapped.IPv4Address = endpoint.IPAMConfig.IPv4Address.String()
				}
				if endpoint.IPAMConfig.IPv6Address.IsValid() {
					mapped.IPv6Address = endpoint.IPAMConfig.IPv6Address.String()
				}
			}
		}
		out[name] = mapped
	}

	return out
}

// Processes is the process snapshot Docker reports for a running container.
type Processes struct {
	// Titles are the column headers Docker returned, in order.
	//
	// Required: true
	Titles []string `json:"titles"`

	// Processes are the rows Docker returned; each row is positional to Titles.
	//
	// Required: true
	Processes [][]string `json:"processes"`
}
