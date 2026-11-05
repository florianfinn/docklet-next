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
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
	"maps"
	"strconv"
	"strings"
	"time"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	imagetypes "github.com/florianfinn/docklet-next/types/image"
)

// ResourceSample is a point-in-time resource usage sample for a container,
// returned with resource-sorted list responses so ordering and displayed
// values come from the same data.
type ResourceSample struct {
	// CPUPercent is the normalized CPU usage percentage (0-100 scale).
	//
	// Required: true
	CPUPercent float64 `json:"cpuPercent"`

	// MemoryUsageBytes is the memory usage in bytes.
	//
	// Required: true
	MemoryUsageBytes uint64 `json:"memoryUsageBytes"`

	// MemoryLimitBytes is the memory limit in bytes.
	//
	// Required: true
	MemoryLimitBytes uint64 `json:"memoryLimitBytes"`

	// SampleTime is when the sample was taken.
	//
	// Required: true
	SampleTime time.Time `json:"sampleTime"`
}

// Summary represents a container summary.
type Summary struct {
	OriginFields
	ImagePlatform string `json:"imagePlatform" doc:"Image manifest platform or unknown when the daemon does not provide it"`
	// ID is the unique identifier of the container.
	//
	// Required: true
	ID string `json:"id"`

	// Names is a list of names for the container.
	//
	// Required: true
	Names []string `json:"names"`

	// Image used by the container.
	//
	// Required: true
	Image string `json:"image"`

	// ImageID is the ID of the image.
	//
	// Required: true
	ImageID string `json:"imageId"`

	// Command running in the container.
	//
	// Required: true
	Command string `json:"command"`

	// Created is the Unix timestamp when the container was created.
	//
	// Required: true
	Created int64 `json:"created"`

	// Ports exposed by the container.
	//
	// Required: true
	Ports []Port `json:"ports"`

	// Labels contains user-defined metadata.
	//
	// Required: true
	Labels map[string]string `json:"labels"`

	// State is the current state of the container.
	//
	// Required: true
	State string `json:"state"`

	// Status provides a human-readable status.
	//
	// Required: true
	Status string `json:"status"`

	// HostConfig contains host configuration.
	//
	// Required: true
	HostConfig HostConfig `json:"hostConfig"`

	// NetworkSettings contains network configuration.
	//
	// Required: true
	NetworkSettings NetworkSettings `json:"networkSettings"`

	// Mounts lists volume mounts.
	//
	// Required: true
	Mounts []Mount `json:"mounts"`

	// IconLightURL is the resolved light icon URL for dark themes.
	//
	// Required: false
	IconLightURL string `json:"iconLightUrl,omitempty"`

	// IconDarkURL is the resolved dark icon URL for light themes.
	//
	// Required: false
	IconDarkURL string `json:"iconDarkUrl,omitempty"`

	// UpdateStrategy is the strategy resolved by the updater for this image and policy.
	UpdateStrategy string `json:"updateStrategy,omitempty"`

	// UpdateInfo contains image update information for this container.
	//
	// Required: false
	UpdateInfo *imagetypes.UpdateInfo `json:"updateInfo,omitempty"`

	// RedeployDisabled indicates whether redeploy actions are disabled for this container.
	//
	// Required: false
	RedeployDisabled bool `json:"redeployDisabled,omitempty"`

	// AutoUpdateEnabled reports whether the container is eligible for auto-update
	// under its updater label and the excluded-containers setting. It does not
	// reflect whether the auto-update scheduler itself is enabled.
	//
	// Required: true
	AutoUpdateEnabled bool `json:"autoUpdateEnabled"`

	// Hidden indicates whether the container is hidden from lists and dashboard counts.
	//
	// Required: false
	Hidden bool `json:"hidden,omitempty"`

	// ResourceSample holds the resource usage sample collected for a
	// resource-sorted list request. Nil when the request was not resource-sorted
	// or the sample was unavailable (stopped container or collection failure).
	//
	// Required: false
	ResourceSample *ResourceSample `json:"resourceSample,omitempty"`
}

// ComposeInfo contains Docker Compose project information extracted from container labels.
type ComposeInfo struct {
	// ProjectName is the name of the Docker Compose project.
	//
	// Required: true
	ProjectName string `json:"projectName"`

	// ServiceName is the name of the service within the Compose project.
	//
	// Required: true
	ServiceName string `json:"serviceName"`

	// WorkingDir is the working directory of the Compose project.
	//
	// Required: false
	WorkingDir string `json:"workingDir,omitempty"`

	// ConfigFiles is the list of Compose config file paths for the project.
	//
	// Required: false
	ConfigFiles string `json:"configFiles,omitempty"`
}

// SummaryGroup represents a group of container summaries.
type SummaryGroup struct {
	// GroupName is the group label, such as a compose project name.
	//
	// Required: true
	GroupName string `json:"groupName"`

	// Items contains the container summaries in the group.
	//
	// Required: true
	Items []Summary `json:"items"`
}

// Details represents detailed container information.
type Details struct {
	OriginFields
	ImagePlatform string `json:"imagePlatform" doc:"Image manifest platform or unknown when the daemon does not provide it"`
	// ID is the unique identifier of the container.
	//
	// Required: true
	ID string `json:"id"`

	// Name of the container.
	//
	// Required: true
	Name string `json:"name"`

	// Image used by the container.
	//
	// Required: true
	Image string `json:"image"`

	// ImageID is the ID of the image.
	//
	// Required: true
	ImageID string `json:"imageId"`

	// Created is when the container was created.
	//
	// Required: true
	Created string `json:"created"`

	// State contains the container's current state.
	//
	// Required: true
	State State `json:"state"`

	// Config contains container configuration.
	//
	// Required: true
	Config Config `json:"config"`

	// HostConfig contains host-level configuration.
	//
	// Required: true
	HostConfig HostConfig `json:"hostConfig"`

	// NetworkSettings contains network configuration.
	//
	// Required: true
	NetworkSettings NetworkSettings `json:"networkSettings"`

	// Ports exposed by the container.
	//
	// Required: true
	Ports []Port `json:"ports"`

	// Mounts lists volume mounts.
	//
	// Required: true
	Mounts []Mount `json:"mounts"`

	// Labels contains user-defined metadata.
	//
	// Required: false
	Labels map[string]string `json:"labels,omitempty"`

	// ComposeInfo contains Docker Compose project information.
	// Only present if container is part of a Compose project.
	//
	// Required: false
	ComposeInfo *ComposeInfo `json:"composeInfo,omitempty"`

	// IconLightURL is the resolved light icon URL for dark themes.
	//
	// Required: false
	IconLightURL string `json:"iconLightUrl,omitempty"`

	// IconDarkURL is the resolved dark icon URL for light themes.
	//
	// Required: false
	IconDarkURL string `json:"iconDarkUrl,omitempty"`

	// RedeployDisabled indicates whether redeploy actions are disabled for this container.
	//
	// Required: false
	RedeployDisabled bool `json:"redeployDisabled,omitempty"`

	// AutoUpdateEnabled reports whether the container is eligible for auto-update
	// under its updater label and the excluded-containers setting. It does not
	// reflect whether the auto-update scheduler itself is enabled.
	//
	// Required: true
	AutoUpdateEnabled bool `json:"autoUpdateEnabled"`

	// UpdateInfo is the stored update check matching this container's current policy.
	//
	// Required: false
	UpdateInfo *imagetypes.UpdateInfo `json:"updateInfo,omitempty"`

	// ActivityID is the background activity that tracked the action returning these details.
	//
	// Required: false
	ActivityID *string `json:"activityId,omitempty"`
}

// Created represents a newly created container.
type Created struct {
	// ID is the unique identifier of the container.
	//
	// Required: true
	ID string `json:"id"`

	// Name of the container.
	//
	// Required: true
	Name string `json:"name"`

	// Image used by the container.
	//
	// Required: true
	Image string `json:"image"`

	// Status of the container.
	//
	// Required: true
	Status string `json:"status"`

	// Created is when the container was created.
	//
	// Required: true
	Created string `json:"created"`
}

// NewSummary creates a Summary from a docker container.Summary.
func NewSummary(c container.Summary) Summary {
	names := make([]string, 0, len(c.Names))
	for _, name := range c.Names {
		names = append(names, strings.TrimPrefix(name, "/"))
	}

	ports := make([]Port, 0, len(c.Ports))
	for _, p := range c.Ports {
		ports = append(ports, Port{
			IP:          p.IP.String(),
			PrivatePort: int(p.PrivatePort),
			PublicPort:  int(p.PublicPort),
			Type:        p.Type,
		})
	}

	mounts := make([]Mount, 0, len(c.Mounts))
	for _, m := range c.Mounts {
		mounts = append(mounts, Mount{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		})
	}

	networks := map[string]NetworkEndpoint{}
	if c.NetworkSettings != nil && c.NetworkSettings.Networks != nil {
		for name, n := range c.NetworkSettings.Networks {
			networks[name] = mapEndpointSettings(n)
		}
	}

	return Summary{
		ID:            c.ID,
		Names:         names,
		Image:         c.Image,
		ImageID:       c.ImageID,
		ImagePlatform: manifestPlatform(c.ImageManifestDescriptor),
		Command:       c.Command,
		Created:       c.Created,
		Ports:         ports,
		Labels:        c.Labels,
		State:         string(c.State),
		Status:        c.Status,
		HostConfig: HostConfig{
			NetworkMode: c.HostConfig.NetworkMode,
		},
		NetworkSettings: NetworkSettings{
			Networks: networks,
		},
		Mounts: mounts,
	}
}

// NewDetails creates a Details from a docker container.InspectResponse.
func NewDetails(c *container.InspectResponse) Details {
	cfg, labels, imageName := mapInspectConfig(c.Config)

	return Details{
		ID:            c.ID,
		Name:          strings.TrimPrefix(c.Name, "/"),
		Image:         imageName,
		ImageID:       c.Image,
		ImagePlatform: manifestPlatform(c.ImageManifestDescriptor),
		Created:       c.Created,
		State:         mapInspectState(c.State),
		Config:        cfg,
		HostConfig:    mapInspectHostConfig(c.HostConfig),
		NetworkSettings: NetworkSettings{
			Networks: mapInspectNetworks(c.NetworkSettings),
		},
		Ports:       mapInspectPorts(c.NetworkSettings),
		Mounts:      mapInspectMounts(c.Mounts),
		Labels:      labels,
		ComposeInfo: mapComposeInfo(labels),
	}
}

func mapInspectPorts(networkSettings *container.NetworkSettings) []Port {
	ports := make([]Port, 0)
	if networkSettings == nil || networkSettings.Ports == nil {
		return ports
	}

	for p, bindings := range networkSettings.Ports {
		privatePort := int(p.Num())
		typ := string(p.Proto())

		// When no host bindings exist, still include the private port.
		if len(bindings) == 0 {
			ports = append(ports, Port{
				PrivatePort: privatePort,
				Type:        typ,
			})
			continue
		}

		for _, b := range bindings {
			pub, _ := strconv.Atoi(b.HostPort)
			ports = append(ports, Port{
				IP:          b.HostIP.String(),
				PrivatePort: privatePort,
				PublicPort:  pub,
				Type:        typ,
			})
		}
	}

	return ports
}

func mapInspectMounts(mountPoints []container.MountPoint) []Mount {
	mounts := make([]Mount, 0, len(mountPoints))
	for _, m := range mountPoints {
		mounts = append(mounts, Mount{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		})
	}

	return mounts
}

func mapInspectNetworks(networkSettings *container.NetworkSettings) map[string]NetworkEndpoint {
	networks := map[string]NetworkEndpoint{}
	if networkSettings == nil || networkSettings.Networks == nil {
		return networks
	}

	for name, n := range networkSettings.Networks {
		networks[name] = mapEndpointSettings(n)
	}

	return networks
}

func mapInspectHostConfig(hostConfig *container.HostConfig) HostConfig {
	if hostConfig == nil {
		return HostConfig{}
	}

	return HostConfig{
		RestartPolicy: string(hostConfig.RestartPolicy.Name),
		Privileged:    hostConfig.Privileged,
		AutoRemove:    hostConfig.AutoRemove,
		NanoCPUs:      hostConfig.NanoCPUs,
		Memory:        hostConfig.Memory,
	}
}

func mapInspectConfig(config *container.Config) (Config, map[string]string, string) {
	labels := map[string]string{}
	if config == nil {
		return Config{}, labels, ""
	}

	cfg := Config{
		Env:        append([]string{}, config.Env...),
		Cmd:        append([]string{}, config.Cmd...),
		Entrypoint: append([]string{}, config.Entrypoint...),
		WorkingDir: config.WorkingDir,
		User:       config.User,
	}

	if hc := config.Healthcheck; hc != nil {
		cfg.Healthcheck = &Healthcheck{
			Test:          append([]string{}, hc.Test...),
			Interval:      int64(hc.Interval),
			Timeout:       int64(hc.Timeout),
			StartPeriod:   int64(hc.StartPeriod),
			StartInterval: int64(hc.StartInterval),
			Retries:       hc.Retries,
		}
	}

	if config.Labels != nil {
		maps.Copy(labels, config.Labels)
	}

	return cfg, labels, config.Image
}

func mapInspectState(state *container.State) State {
	if state == nil {
		return State{}
	}

	mappedState := State{
		Status:     string(state.Status),
		Running:    state.Running,
		ExitCode:   state.ExitCode,
		StartedAt:  state.StartedAt,
		FinishedAt: state.FinishedAt,
	}

	if state.Health != nil {
		mappedState.Health = mapInspectHealth(state.Health)
	}

	return mappedState
}

func mapInspectHealth(health *container.Health) *Health {
	log := make([]HealthLogEntry, 0, len(health.Log))
	for _, entry := range health.Log {
		if entry == nil {
			continue
		}

		log = append(log, HealthLogEntry{
			Start:    formatTimeOrEmpty(entry.Start),
			End:      formatTimeOrEmpty(entry.End),
			ExitCode: entry.ExitCode,
			Output:   entry.Output,
		})
	}

	return &Health{
		Status:        string(health.Status),
		FailingStreak: health.FailingStreak,
		Log:           log,
	}
}

func formatTimeOrEmpty(t time.Time) string {
	if t.IsZero() {
		return ""
	}
	return t.Format("2006-01-02T15:04:05.999999999Z07:00")
}

func mapComposeInfo(labels map[string]string) *ComposeInfo {
	projectName, hasProject := labels["com.docker.compose.project"]
	if !hasProject {
		return nil
	}

	serviceName, hasService := labels["com.docker.compose.service"]
	if !hasService {
		return nil
	}

	composeInfo := &ComposeInfo{
		ProjectName: projectName,
		ServiceName: serviceName,
	}
	if workingDir, ok := labels["com.docker.compose.project.working_dir"]; ok {
		composeInfo.WorkingDir = workingDir
	}
	if configFiles, ok := labels["com.docker.compose.project.config_files"]; ok {
		composeInfo.ConfigFiles = configFiles
	}

	return composeInfo
}

func mapEndpointSettings(n *network.EndpointSettings) NetworkEndpoint {
	if n == nil {
		return NetworkEndpoint{}
	}

	var driverOpts map[string]string
	if n.DriverOpts != nil {
		driverOpts = n.DriverOpts
	}

	gateway := ""
	if n.Gateway.IsValid() {
		gateway = n.Gateway.String()
	}

	ipAddress := ""
	if n.IPAddress.IsValid() {
		ipAddress = n.IPAddress.String()
	}

	ipv6Gateway := ""
	if n.IPv6Gateway.IsValid() {
		ipv6Gateway = n.IPv6Gateway.String()
	}

	globalIPv6Address := ""
	if n.GlobalIPv6Address.IsValid() {
		globalIPv6Address = n.GlobalIPv6Address.String()
	}

	return NetworkEndpoint{
		IPAMConfig:          n.IPAMConfig,
		Links:               n.Links,
		Aliases:             n.Aliases,
		MacAddress:          n.MacAddress.String(),
		DriverOpts:          driverOpts,
		GwPriority:          n.GwPriority,
		NetworkID:           n.NetworkID,
		EndpointID:          n.EndpointID,
		Gateway:             gateway,
		IPAddress:           ipAddress,
		IPPrefixLen:         n.IPPrefixLen,
		IPv6Gateway:         ipv6Gateway,
		GlobalIPv6Address:   globalIPv6Address,
		GlobalIPv6PrefixLen: n.GlobalIPv6PrefixLen,
		DNSNames:            n.DNSNames,
	}
}

func manifestPlatform(descriptor *ocispec.Descriptor) string {
	if descriptor == nil || descriptor.Platform == nil || descriptor.Platform.OS == "" || descriptor.Platform.Architecture == "" {
		return "unknown"
	}
	p := descriptor.Platform
	value := p.OS + "/" + p.Architecture
	if p.Variant != "" {
		value += "/" + p.Variant
	}
	return value
}
