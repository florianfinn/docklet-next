// Source: docklet-next/issues/474
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original check that production wiring registers the proxy engines.

package di

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
)

func TestRegisterEnginesAddsEveryKind(t *testing.T) {
	m := &proxy.Module{Agent: &proxy.Agent{Engines: proxy.Engines{}}, Runtime: &proxy.DockerRuntime{}}
	registerEngines(m)
	_, err := m.Agent.Engines.For(dto.Caddy)
	require.NoError(t, err)
}
