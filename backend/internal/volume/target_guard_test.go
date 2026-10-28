// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original target volume hiding, redaction and destination contention through real HTTP/Edge handlers.

package volume

import (
	"encoding/json"
	"strings"
	"testing"

	dto "github.com/florianfinn/docklet-next/types/backup"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	mobyvolume "github.com/moby/moby/api/types/volume"
	"github.com/stretchr/testify/require"
)

func TestTargetVolumeHiddenAndRedactedThroughHTTPAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			secret := strings.Repeat("s", 30)
			target := pruneVolume("target-mount")
			target.Labels = map[string]string{dto.TargetVolumeLabel: "target"}
			target.Options = map[string]string{"type": "cifs", "o": "addr=nas.example.com,password=" + secret}
			projected := volumetypes.NewSummary(target)
			encoded, err := json.Marshal(projected)
			require.NoError(t, err)
			require.NotContains(t, string(encoded), secret)
			require.NotContains(t, projected.Options, "o")
			require.Contains(t, target.Options["o"], secret, "DTO projection must not alter daemon state")
			f := &createDockerFixture{volumes: map[string]mobyvolume.Volume{target.Name: target}}
			svc := f.service(t)
			svc.settingsService = usageSettings(t)
			call := pruneTransport(t, mode, pruneRouter(t, svc))
			for _, query := range []string{"", "includeInternal=true"} {
				status, body := call("GET", "/api/environments/0/volumes", query, nil)
				require.Equal(t, 200, status, string(body))
				require.NotContains(t, string(body), target.Name)
				require.NotContains(t, string(body), secret)
			}
			for _, request := range []struct {
				method, path string
				body         []byte
			}{{"GET", "/api/environments/0/volumes/target-mount", nil}, {"DELETE", "/api/environments/0/volumes/target-mount", nil}, {"POST", "/api/environments/0/volumes/target-mount/rename", []byte(`{"name":"new-name"}`)}} {
				status, body := call(request.method, request.path, "", request.body)
				require.Equal(t, 404, status, string(body))
				require.NotContains(t, string(body), secret)
				require.Contains(t, string(body), "volume.not-found")
			}
			create := pruneTransport(t, mode, createRouter(t, svc))
			status, body := create("POST", "/api/environments/0/volumes", "", []byte(`{"name":"target-mount","driver":"local"}`))
			require.Equal(t, 404, status, string(body))
			require.Contains(t, string(body), "volume.not-found")
			require.Zero(t, f.creates)
		})
	}
}
func TestCreateAndRenameRefuseHeldDestinationBeforeDocker(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			f := &createDockerFixture{volumes: map[string]mobyvolume.Volume{}, inspectStatus: 500}
			svc := f.service(t)
			unlock := svc.workspaceLocks.Lock("new-data")
			defer unlock()
			create := pruneTransport(t, mode, createRouter(t, svc))
			status, body := create("POST", "/api/environments/0/volumes", "", []byte(`{"name":"new-data","driver":"local"}`))
			require.Equal(t, 409, status, string(body))
			rename := pruneTransport(t, mode, pruneRouter(t, svc))
			status, body = rename("POST", "/api/environments/0/volumes/source/rename", "", []byte(`{"name":"new-data"}`))
			require.Equal(t, 409, status, string(body))
			require.Zero(t, f.reads, "busy destination must refuse before Docker reads")
			require.Zero(t, f.creates)
		})
	}
}
