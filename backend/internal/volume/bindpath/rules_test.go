// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package bindpath

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func binding(id, source string, labels map[string]string) container.Summary {
	return container.Summary{ID: id, Labels: labels, Mounts: []container.MountPoint{{Type: mount.TypeBind, Source: source, Destination: "/data"}}}
}
func TestBindSystemAndDockerOverlap(t *testing.T) {
	for _, tc := range []struct{ source, root, reason string }{{"/etcetera", "", ""}, {"/etc/x", "", "system"}, {"/var/lib", "", "docker"}, {"/srv", "/srv/engine", "docker"}, {"/srv/engine/x", "/srv/engine", "docker"}, {"/srv2", "/srv/engine", ""}} {
		err := Rules(tc.source, tc.root, nil, nil, true)
		if tc.reason == "" {
			require.NoError(t, err, tc.source)
		} else {
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "volume.bind.path-denied", p.Code)
			require.Equal(t, tc.reason, p.Reason)
		}
	}
	require.True(t, Overlap("/srv/app", "/srv/app/data"))
	require.True(t, Overlap("/srv", "/srv/app"))
	require.False(t, Overlap("/srv/app", "/srv/app2"))
}
func TestBindDockletTargetsAndObservedAdmission(t *testing.T) {
	self := binding("hub", "/srv/docklet", map[string]string{"com.getarcaneapp.arcane": "true"})
	helper := binding("helper", "/srv/app", volumehelper.Labels())
	workload := binding("app", "/srv/app", nil)
	for _, source := range []string{"/srv", "/srv/docklet", "/srv/docklet/data"} {
		require.Error(t, Rules(source, "", []container.Summary{self}, nil, true))
	}
	for _, source := range []string{"/srv", "/srv/target", "/srv/target/archive"} {
		require.Error(t, Rules(source, "", nil, []string{"/srv/target"}, true))
	}
	require.EqualError(t, Rules("/srv/app", "", []container.Summary{helper}, nil, false), "volume.bind.not-mounted")
	require.NoError(t, Rules("/srv/app", "", []container.Summary{helper, workload}, nil, false))
	require.NoError(t, Rules("/srv/app", "", []container.Summary{helper}, nil, true))
	require.NoError(t, Rules("/srv/app", "", nil, nil, true), "retained rows keep the key usable")
	require.True(t, Uses(workload, "/srv", true))
	require.False(t, Uses(workload, "/srv", false))
}
func TestBindWitnessKindAndStructuredMount(t *testing.T) {
	for _, tc := range []struct {
		output, want string
		fail         bool
	}{{"directory", "", false}, {"file", "", false}, {"unsupported", "volume.bind.unsupported-type", false}, {"", "volume.helper-unavailable", true}, {"invalid", "volume.helper-unavailable", false}} {
		t.Run(tc.output, func(t *testing.T) {
			removed := false
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case strings.HasSuffix(r.URL.Path, "/containers/create"):
					var body struct{ HostConfig container.HostConfig }
					require.NoError(t, json.NewDecoder(r.Body).Decode(&body))
					require.Empty(t, body.HostConfig.Binds)
					require.Equal(t, []mount.Mount{{Type: mount.TypeBind, Source: "/srv/app", Target: "/source", ReadOnly: true}}, body.HostConfig.Mounts)
					_, _ = w.Write([]byte(`{"Id":"witness"}`))
				case r.Method == "DELETE":
					removed = true
					w.WriteHeader(204)
				case strings.HasSuffix(r.URL.Path, "/start"):
					w.WriteHeader(204)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(500)
				}
			}))
			defer server.Close()
			cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"), client.WithHTTPClient(server.Client()))
			require.NoError(t, err)
			defer cli.Close()
			kind, err := Witness(t.Context(), cli, "tools:1.0.0", "/srv/app", func(context.Context, string, string, []string) (string, string, error) {
				if tc.fail {
					return "", "", errors.New("exec failed")
				}
				return tc.output, "", nil
			})
			require.True(t, removed)
			if tc.want == "" {
				require.NoError(t, err)
				require.Equal(t, tc.output, kind)
			} else {
				require.EqualError(t, err, tc.want)
			}
		})
	}
	require.Equal(t, "/volume/app.conf", SourceMount("/srv/app.conf", "file", true).Target)
	require.Equal(t, "/volume", SourceMount("/srv/app", "directory", false).Target)
}
