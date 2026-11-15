// Source: getarcaneapp/arcane/types/features/features.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed Swarm; optional-module registry and decoded route ownership (#472).

// Package features defines runtime features and their settings contracts.
package features

import "strings"

// ID identifies a feature independently of permissions and build options.
type ID string

// Kind separates optional modules from existing runtime features.
type Kind string

const (
	Feature         Kind = "feature"
	Module          Kind = "module"
	Proxy           ID   = "proxy"
	ProxySettingKey      = "moduleProxyEnabled"
)

const (
	// VulnerabilityManagement identifies scanning, reports, and scan-based patching.
	VulnerabilityManagement ID = "vulnerabilityManagement"

	// VulnerabilityManagementSettingKey is the persisted toggle for vulnerability management.
	VulnerabilityManagementSettingKey = "featureVulnerabilityManagementEnabled"
)

// Definition connects a runtime feature to its persisted setting and default.
type Definition struct {
	ID             ID
	Kind           Kind
	HubPrefix      string
	HostPrefix     string
	Capability     string
	AgentModule    string
	SettingKey     string
	DefaultEnabled bool
}

var registryInternal = [...]Definition{
	{ID: VulnerabilityManagement, Kind: Feature, SettingKey: VulnerabilityManagementSettingKey, DefaultEnabled: true},
	{ID: Proxy, Kind: Module, SettingKey: ProxySettingKey, HubPrefix: "/api/modules/proxy", HostPrefix: "/api/environments/{id}/modules/proxy", Capability: "module.proxy", AgentModule: "proxy"},
}

// All returns the supported runtime features.
func All() []Definition {
	return append([]Definition{}, registryInternal[:]...)
}

// Lookup returns the definition of a supported runtime feature.
func Lookup(id ID) (Definition, bool) {
	for _, definition := range registryInternal {
		if definition.ID == id {
			return definition, true
		}
	}
	return Definition{}, false
}

// Modules returns optional modules in deterministic registration order.
func Modules() []Definition {
	var modules []Definition
	for _, d := range registryInternal {
		if d.Kind == Module {
			modules = append(modules, d)
		}
	}
	return modules
}

// ForPath classifies the decoded path the router and remote sender actually use.
// Segment boundaries prevent similarly named routes from acquiring module ownership.
func ForPath(decodedPath string) (Definition, bool) {
	if i := strings.IndexAny(decodedPath, "?#"); i >= 0 {
		decodedPath = decodedPath[:i]
	}
	parts := strings.Split(decodedPath, "/")
	var prefix string
	if len(parts) >= 4 && parts[1] == "api" && parts[2] == "modules" {
		prefix = "/api/modules/" + parts[3]
	}
	if len(parts) >= 6 && parts[1] == "api" && parts[2] == "environments" && parts[3] != "" && parts[4] == "modules" {
		prefix = "/api/environments/{id}/modules/" + parts[5]
	}
	for _, d := range Modules() {
		if prefix == d.HubPrefix || prefix == d.HostPrefix {
			return d, true
		}
	}
	return Definition{}, false
}
