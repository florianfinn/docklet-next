// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original transient-source tests from the M27 candidate run (F3).

package proxy

import (
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	dockerinternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/stretchr/testify/require"
)

// A transient Docker failure while classifying a source (a restarting container's
// path stat, the daemon info for bind rules) is an error, never a read-only source;
// only a missing path and a bind-rule refusal are classifications.
func TestSourceTransientErrorsAreNotClassifications(t *testing.T) {
	for _, tt := range []struct {
		name         string
		source       string
		stat, info   int
		wantErr      bool
		wantKind     dto.SourceKind
		wantReadOnly bool
	}{
		{"stat conflict", "/example/binds/proxy", http.StatusConflict, http.StatusOK, true, "", false},
		{"stat missing", "/example/binds/proxy", http.StatusNotFound, http.StatusOK, false, dto.UnknownSourceKind, true},
		{"info failure", "/example/binds/proxy", http.StatusOK, http.StatusInternalServerError, true, "", false},
		{"bind rule refusal", "/etc/proxy", http.StatusOK, http.StatusOK, false, dto.BindSource, true},
		{"bind allowed (helper-only, read-only)", "/example/binds/proxy", http.StatusOK, http.StatusOK, false, dto.BindSource, true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			x := newFixture(t)
			require.NoError(t, x.settings.UpdateSetting(t.Context(), "projectsDirectory", t.TempDir()))
			c := container.InspectResponse{ID: "example-container", Name: "/example-proxy", State: &container.State{Running: true, StartedAt: initialEpoch}, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: tt.source, Destination: "/etc/proxy", RW: true}}}
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				p := regexp.MustCompile(`^/v[0-9.]+`).ReplaceAllString(r.URL.Path, "")
				switch {
				case p == "/_ping":
					w.Header().Set("API-Version", "1.54")
					_, _ = w.Write([]byte("OK"))
				case p == "/info":
					if tt.info != http.StatusOK {
						http.Error(w, `{"message":"daemon busy"}`, tt.info)
						return
					}
					_, _ = w.Write([]byte(`{"DockerRootDir":"/example/docker"}`))
				case p == "/containers/json":
					_ = json.NewEncoder(w).Encode([]container.Summary{{ID: c.ID, Mounts: c.Mounts}}) // the proxy itself uses the bind
				case p == "/containers/example-container/archive":
					if tt.stat != http.StatusOK {
						w.WriteHeader(tt.stat)
						return
					}
					raw, _ := json.Marshal(container.PathStat{Name: "proxy", Mode: os.ModeDir | 0755})
					w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(raw))
				default:
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			d := &DockerRuntime{Docker: dockerinternal.NewDockerClientService(t.Context(), nil, &config.Config{DockerHost: server.URL}, nil), Settings: x.settings, DB: x.db}
			s, err := d.Source(t.Context(), c, c.Mounts[0])
			require.Equal(t, tt.wantErr, err != nil, "error: %v", err)
			if !tt.wantErr {
				require.Equal(t, tt.wantKind, s.Kind)
				require.Equal(t, tt.wantReadOnly, s.ReadOnly)
			}
		})
	}
}
