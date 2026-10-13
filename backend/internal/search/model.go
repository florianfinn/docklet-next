// Source: getarcaneapp/arcane/backend/internal/search/model.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Functional cut: removed template catalogue; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories.

package search

// CustomizeItem represents a customization configuration item
// This struct uses reflection tags to define customization categories and metadata
// Categories must be grouped together and use `catmeta` tag for category-level metadata
//
//nolint:lll // Reflection metadata must remain in single struct-tag literals.
type CustomizeItem struct {
	// Defaults category
	DefaultContainerSettings CustomizeVariable `key:"defaultContainerSettings" meta:"label=Default Container Settings;type=object;keywords=container,default,settings,docker,configuration,runtime;category=defaults;description=Configure default container runtime settings"`
	DefaultNetworkMode       CustomizeVariable `key:"defaultNetworkMode" meta:"label=Default Network Mode;type=select;keywords=network,default,mode,bridge,host,none,container;category=defaults;description=Set the default network mode for containers"`

	// Registries category
	ContainerRegistries CustomizeVariable `key:"containerRegistries" meta:"label=Container Registries;type=array;keywords=registry,docker,images,hub,private,authentication,credentials;category=registries;description=Manage container registry connections" catmeta:"id=registries;title=Container Registries;icon=package;url=/customize/registries;description=Configure container registries and authentication"`
	RegistryCredentials CustomizeVariable `key:"registryCredentials" meta:"label=Registry Credentials;type=secure;keywords=credentials,auth,username,password,token,login,security;category=registries;description=Configure authentication for container registries"`
	RegistryMirrors     CustomizeVariable `key:"registryMirrors" meta:"label=Registry Mirrors;type=array;keywords=mirrors,proxy,cache,performance,cdn,regional;category=registries;description=Configure registry mirrors and proxies"`

	// Git repositories category
}

type CustomizeVariable struct {
	Value string
}
