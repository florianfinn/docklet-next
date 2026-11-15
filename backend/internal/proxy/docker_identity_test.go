// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Real Docker HTTP identity fencing and transient cleanup regression.

package proxy

import (
	"archive/tar"
	"context"
	"encoding/base64"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"
)

type dockerIdentityEngine struct {
	*fakeEngine
	host *DockerRuntime
}

func (e *dockerIdentityEngine) Read(ctx context.Context, i dto.Instance) (dto.ReadResult, error) {
	g, err := e.runtime.Graph(ctx, i)
	r := dto.ReadResult{Files: []dto.File{}}
	for p, h := range g {
		r.Files = append(r.Files, dto.File{Path: p, Hash: h})
	}
	return r, err
}
func (e *dockerIdentityEngine) Mark(ctx context.Context, i Intent) error {
	e.marker++
	w, err := e.host.Writer(ctx, i.Request.Instance)
	if err != nil {
		return err
	}
	return w.Write(ctx, i.MarkerPath, []byte("marker"), 0644)
}
func TestP8RecreationCleansOnlyOwnedTransientFiles(t *testing.T) {
	x := newFixture(t)
	root := t.TempDir()
	require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", root))
	require.NoError(t, os.WriteFile(filepath.Join(root, "route.yml"), []byte("old"), 0644))
	var mu sync.Mutex
	c := container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: root, Destination: "/etc/proxy", RW: true}}}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
		switch {
		case p == "/_ping":
			w.Header().Set("API-Version", "1.54")
			_, _ = w.Write([]byte("OK"))
		case strings.HasSuffix(p, "/json"):
			_ = json.NewEncoder(w).Encode(c)
		case strings.HasSuffix(p, "/archive"):
			source := r.URL.Query().Get("path")
			mode := os.FileMode(0644)
			if source == "/etc/proxy" {
				mode |= os.ModeDir
			}
			raw, _ := json.Marshal(container.PathStat{Name: path.Base(source), Mode: mode})
			w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
			if r.Method == "HEAD" {
				return
			}
			b, err := os.ReadFile(filepath.Join(root, strings.TrimPrefix(source, "/etc/proxy/")))
			if err != nil {
				w.WriteHeader(404)
				return
			}
			tw := tar.NewWriter(w)
			_ = tw.WriteHeader(&tar.Header{Name: path.Base(source), Typeflag: tar.TypeReg, Mode: 0644, Size: int64(len(b))})
			_, _ = tw.Write(b)
			_ = tw.Close()
		default:
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
	e := &dockerIdentityEngine{fakeEngine: x.e, host: d}
	d.Engines = Engines{dto.Traefik: e}
	x.a.Runtime = d
	x.a.Engines = d.Engines
	x.request.Instance.Source = dto.Source{Kind: dto.ProjectSource, Canonical: root, MountTarget: "/etc/proxy"}
	e.observe = func() Observation {
		if e.marker == 0 {
			return Observation{NeedsMarker: true}
		}
		mu.Lock()
		c.ID = "example-recreated"
		c.State.StartedAt = nextEpoch
		mu.Unlock()
		return Observation{State: dto.Verified, Evidence: dto.Load}
	}
	o, err := x.a.Apply(t.Context(), x.request)
	require.NoError(t, err)
	require.Equal(t, dto.AppliedUnverified, o.State)
	require.Equal(t, dto.ContainerChanged, o.Reason)
	b, err := os.ReadFile(filepath.Join(root, "route.yml"))
	require.NoError(t, err)
	require.Equal(t, "new", string(b), "identity changes must not restore the published target")
	_, err = os.Stat(filepath.Join(root, "docklet-check-"+x.request.ID+".yml"))
	require.True(t, os.IsNotExist(err), "the operation's marker must be removed after recreation on the same source")
}
