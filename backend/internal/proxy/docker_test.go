// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original hostile-input, epoch and writer proofs.

package proxy

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/proxy/hostio"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"reflect"
	"regexp"
	"strings"
	"sync"
	"testing"
)

func TestP17SourceSelectionAndReinspection(t *testing.T) {
	x := newFixture(t)
	project := t.TempDir()
	for _, tt := range []struct {
		name, source string
		kind         mount.Type
		dir, rw      bool
		want         dto.SourceKind
		writable     bool
	}{
		{"project", project, mount.TypeBind, true, true, dto.ProjectSource, true},
		// Helper-only sources stay read-only in M27 (Q-HELPER-META): metadata cannot be proven.
		{"named-volume", "/example/volumes/data", mount.TypeVolume, true, true, dto.NamedVolume, false},
		{"refused-bind", "/etc/proxy", mount.TypeBind, true, true, dto.BindSource, false},
		{"single-file", project, mount.TypeBind, false, true, dto.SingleFileSource, false},
		// Proxy-side read-only mounts keep the module's writer (Q-SRC): project writer or helper.
		{"proxy-side-readonly-project", project, mount.TypeBind, true, false, dto.ProjectSource, true},
		{"proxy-side-readonly-volume", "/example/volumes/data", mount.TypeVolume, true, false, dto.NamedVolume, false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			mu := sync.Mutex{}
			c := container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: tt.kind, Source: tt.source, Destination: "/etc/proxy", Name: "example-data", RW: tt.rw}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				defer mu.Unlock()
				p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
				switch {
				case p == "/_ping":
					w.Header().Set("API-Version", "1.54")
					_, _ = w.Write([]byte("OK"))
				case p == "/info":
					_, _ = w.Write([]byte(`{"DockerRootDir":"/example/docker"}`))
				case p == "/containers/json":
					_ = json.NewEncoder(w).Encode([]container.Summary{})
				case strings.HasSuffix(p, "/json") && strings.Contains(p, "/containers/"):
					_ = json.NewEncoder(w).Encode(c)
				case strings.HasSuffix(p, "/archive"):
					mode := os.FileMode(0644)
					if tt.dir {
						mode |= os.ModeDir
					}
					raw, _ := json.Marshal(container.PathStat{Name: path.Base(r.URL.Query().Get("path")), Mode: mode})
					w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
				default:
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
			require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", project))
			source, err := d.Source(t.Context(), c, c.Mounts[0])
			require.NoError(t, err)
			require.Equal(t, tt.want, source.Kind)
			if tt.want == dto.NamedVolume {
				require.Equal(t, dto.UnsupportedMetadata, source.Reason)
			}
			i := x.request.Instance
			i.Identity.ContainerID = c.ID
			i.Source = source
			i.Eligibility.Writable = true
			writer, err := d.Writer(t.Context(), i)
			if tt.writable {
				require.NoError(t, err)
				if tt.want == dto.ProjectSource {
					require.IsType(t, hostio.ProjectWriter{}, writer)
				} else {
					require.IsType(t, hostio.HelperWriter{}, writer)
				}
			} else {
				require.ErrorContains(t, err, "source-read-only")
				require.Nil(t, writer)
			}
			mu.Lock()
			c.Mounts[0].Source = "/example/replaced"
			mu.Unlock()
			require.Error(t, d.Check(t.Context(), i), "every entry must re-inspect the mount")
		})
	}
}
func TestP21ProjectionCopiesSharedInventory(t *testing.T) {
	x := newFixture(t)
	project := t.TempDir()
	require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", project))
	shared := []container.Summary{{ID: "example-container", Image: "traefik:3.7.16", Mounts: []container.MountPoint{{Destination: "/z"}, {Destination: "/a"}}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
		switch {
		case p == "/_ping":
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
		case p == "/containers/json":
			_ = json.NewEncoder(w).Encode(shared)
		case strings.HasSuffix(p, "/json"):
			_ = json.NewEncoder(w).Encode(container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: project, Destination: "/etc/proxy", RW: true}}})
		case strings.HasSuffix(p, "/archive"):
			raw, _ := json.Marshal(container.PathStat{Name: "proxy", Mode: os.ModeDir | 0755})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
	rows, err := d.Docker.ListContainers(t.Context())
	require.NoError(t, err)
	before := append([]container.MountPoint(nil), rows[0].Mounts...)
	var wg sync.WaitGroup
	for range 30 {
		wg.Go(func() {
			out, err := d.ProjectDiscovery(context.Background(), rows)
			require.NoError(t, err)
			require.Len(t, out, 1)
		})
	}
	wg.Wait()
	require.True(t, reflect.DeepEqual(before, rows[0].Mounts), "shared coalesced rows must remain unchanged")
}

func TestImageKindIgnoresRegistryPorts(t *testing.T) {
	for image, want := range map[string]dto.Kind{
		"traefik:3.7.16":                 dto.Traefik,
		"docker.io/library/caddy:2.10.2": dto.Caddy,
		// Measured sandbox mirror form: registry port plus digest.
		"127.0.0.1:55413/fixtures/m27/traefik@sha256:" + strings.Repeat("a", 64): dto.Traefik,
		"registry.example.com:5000/proxy/caddy:2.10.2":                           dto.Caddy,
		"localhost:5000/traefik":                                                 dto.Traefik,
		"caddy@sha256:" + strings.Repeat("b", 64):                                dto.Caddy,
		"registry.example.com:5000/traefik-proxy:1":                              "",
		"registry.example.com:5000/other:traefik":                                "",
		"sha256:" + strings.Repeat("c", 64):                                      "",
	} {
		require.Equal(t, want, imageKind(image), image)
	}
}
