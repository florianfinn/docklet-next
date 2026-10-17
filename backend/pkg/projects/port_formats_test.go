// Source: docklet-next/issues/85
// Revision: 37ddf907edabe127fe061523c7aba1c764d87d85
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original port publisher regression coverage.

package projects

import (
	"github.com/docker/compose/v5/pkg/api"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestD6PortFormatsMergeDuplicatePublishers(t *testing.T) {
	expected := []string{"8080:80/tcp", "8080:80/udp"}
	require.Equal(t, expected, FormatPorts([]api.PortPublisher{{PublishedPort: 8080, TargetPort: 80, Protocol: "tcp"}, {PublishedPort: 8080, TargetPort: 80, Protocol: "udp"}, {PublishedPort: 8080, TargetPort: 80, Protocol: "tcp"}}))
	require.Equal(t, expected, FormatDockerPorts([]container.PortSummary{{PublicPort: 8080, PrivatePort: 80, Type: "tcp"}, {PublicPort: 8080, PrivatePort: 80, Type: "udp"}, {PublicPort: 8080, PrivatePort: 80, Type: "tcp"}}))
}
