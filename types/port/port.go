// Source: getarcaneapp/arcane/types/port/port.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package port

type PortMapping struct {
	// ID is the stable unique identifier for the mapping row.
	//
	// Required: true
	ID string `json:"id"`

	// ContainerID is the owning container ID.
	//
	// Required: true
	ContainerID string `json:"containerId"`

	// ContainerName is the primary container name.
	//
	// Required: true
	ContainerName string `json:"containerName"`

	// HostIP is the host interface IP when the port is published.
	//
	// Required: false
	HostIP string `json:"hostIp,omitempty"`

	// HostPort is the published port on the host.
	//
	// Required: false
	HostPort int `json:"hostPort,omitempty"`

	// ContainerPort is the exposed port inside the container.
	//
	// Required: true
	ContainerPort int `json:"containerPort"`

	// Protocol is the transport protocol.
	//
	// Required: true
	Protocol string `json:"protocol"`

	// IsPublished indicates whether the port is bound on the host.
	//
	// Required: true
	IsPublished bool `json:"isPublished"`
}
