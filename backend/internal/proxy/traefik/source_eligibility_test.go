// Source: docklet-next/issues/528
// Revision: 54fd54b6bf259e09c1d4fa66ab1cd7ebdebf09e6
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 setup eligibility and revalidation through the real Docker seam (U-3, U-4).

package traefik

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/modules"
	"github.com/florianfinn/docklet-next/backend/internal/proxy"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

// eligibilityModule wires a hub+agent module on the local environment over the seam.
func eligibilityModule(t *testing.T, x *seam) *proxy.Module {
	t.Helper()
	require.NoError(t, x.d.Settings.UpdateSetting(t.Context(), features.ProxySettingKey, "true"))
	a, err := proxy.NewAgent(x.db.DB, x.d, x.d.Engines)
	require.NoError(t, err)
	mods := modules.New(x.d.Settings)
	m := &proxy.Module{Agent: a, Hub: &proxy.Hub{DB: x.db.DB, Modules: mods}, Runtime: x.d, Modules: mods, Settings: x.d.Settings}
	m.Transport = &proxy.HostTransport{Local: m}
	// Configuration read-only for the proxy itself, writable for Docklet; a second, unrelated project source.
	require.NoError(t, os.MkdirAll(filepath.Join(x.root, "other"), 0o755))
	require.NoError(t, os.MkdirAll(filepath.Join(x.root, "srv", "other"), 0o755))
	x.inspect.Mounts = []container.MountPoint{
		{Source: x.root, Destination: "/etc/traefik", Type: mount.TypeBind, RW: false},
		{Source: filepath.Join(x.root, "other"), Destination: "/srv/other", Type: mount.TypeBind, RW: true},
	}
	x.containers = []container.Summary{{ID: x.i.Identity.ContainerID, Image: "traefik:v3.7", Names: []string{"/example-proxy"}}}
	return m
}

func sourceAt(t *testing.T, rows []dto.Discovery, target string) dto.Source {
	t.Helper()
	require.Len(t, rows, 1)
	for _, s := range rows[0].Sources {
		if s.MountTarget == target {
			require.NotNil(t, s.Eligibility, target)
			return s
		}
	}
	t.Fatalf("no source at %s", target)
	return dto.Source{}
}

func TestM28SetupEligibilityFollowsTheFileProviderDirectory(t *testing.T) {
	x := fixture(t)
	m := eligibilityModule(t, x)
	rows, err := m.DiscoverHost(t.Context())
	require.NoError(t, err)

	config := sourceAt(t, rows, "/etc/traefik")
	require.Equal(t, dto.ProjectSource, config.Kind)
	require.Equal(t, dto.MountReadOnly, config.MountMode, "proxy-ro mount is display only")
	require.Equal(t, dto.SourceEligibility{Selectable: true, RouteDirectory: &dto.RouteDirectoryHint{Value: "dynamic", Fixed: true}}, *config.Eligibility)

	other := sourceAt(t, rows, "/srv/other")
	require.Equal(t, "other", other.ProjectPath)
	require.Equal(t, dto.MountReadWrite, other.MountMode)
	require.Equal(t, dto.SourceEligibility{Reason: dto.ProviderDirectoryElsewhere}, *other.Eligibility)

	x.inspect.Config.Cmd = []string{"--providers.file.directory=/etc/traefik", "--entrypoints.web.address=:80"}
	rows, err = m.DiscoverHost(t.Context())
	require.NoError(t, err)
	require.Equal(t, &dto.RouteDirectoryHint{Value: ".", Fixed: true}, sourceAt(t, rows, "/etc/traefik").Eligibility.RouteDirectory)
}

func requireReason(t *testing.T, err error, code dto.Reason) {
	t.Helper()
	var p *dto.Problem
	require.True(t, errors.As(err, &p), "%v", err)
	require.Equal(t, code, p.Code)
}

func TestM28ServerRevalidatesSourceAndRouteDirectory(t *testing.T) {
	x := fixture(t)
	m := eligibilityModule(t, x)
	create := func(mountTarget, routes string) (dto.Instance, error) {
		return m.CreateInstance(t.Context(), dto.CreateInstanceRequest{EnvironmentID: "0", Kind: dto.Traefik, ContainerID: x.i.Identity.ContainerID, MountTarget: mountTarget, Name: "example", RouteDirectory: routes})
	}

	_, err := create("/srv/other", "routes")
	requireReason(t, err, dto.SourceIneligible)
	_, err = create("/etc/traefik", "other")
	requireReason(t, err, dto.RouteDirectoryMismatch)
	none, err := m.Instances(t.Context())
	require.NoError(t, err)
	require.Empty(t, none, "a refused create stores nothing")

	i, err := create("/etc/traefik", "/etc/traefik/dynamic")
	require.NoError(t, err, "the fixed directory may be given absolute")
	_, err = m.UpdateInstance(t.Context(), i.ID, dto.Settings{Name: "example", RouteDirectory: "elsewhere"})
	requireReason(t, err, dto.RouteDirectoryMismatch)
	stored, err := m.Instance(t.Context(), i.ID)
	require.NoError(t, err)
	require.Equal(t, "/etc/traefik/dynamic", stored.RouteDirectory)
	_, err = m.UpdateInstance(t.Context(), i.ID, dto.Settings{Name: "renamed", RouteDirectory: "dynamic"})
	require.NoError(t, err)
}
