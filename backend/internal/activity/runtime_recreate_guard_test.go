// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original fresh public origin lock proof across all transports.

package activity_test

import (
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/container/children/origin"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/cgroup"
)

func TestRecreateFreshAdmissionOriginLocksAllTransports(t *testing.T) {
	for _, mode := range []string{"local", "direct", "edge"} {
		t.Run(mode, func(t *testing.T) {
			hub := activationApp(t, false, "")
			executor, environment := hub, "0"
			if mode != "local" {
				executor = activationApp(t, true, uuid.NewString())
				environment = activationRemote(t, hub, executor, mode)
			}
			daemon := newRecreateHTTP(t)
			executor.services.http.onCall = daemon.serve
			for _, tc := range []struct {
				name   string
				labels map[string]string
				manual bool
			}{
				{name: "compose", labels: map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "app"}},
				{name: "unraid", labels: map[string]string{"net.unraid.docker.managed": ""}},
				{name: "unknown", labels: map[string]string{"io.kubernetes.pod.name": "app"}},
				{name: "unconfirmed", labels: map[string]string{"io.kubernetes.pod.name": "app"}, manual: true},
				{name: "self", labels: map[string]string{"com.getarcaneapp.arcane": "true"}},
			} {
				t.Run(tc.name, func(t *testing.T) {
					id := runtimeContainerID
					if tc.name == "self" {
						if current, err := cgroup.CurrentContainerID(); err == nil && current != "" && len(current) <= 64 {
							id = current + strings.Repeat("a", 64-len(current))
						}
					}
					daemon.mu.Lock()
					daemon.containers = map[string]recreateObserved{id: {ID: id, Name: "app", Labels: tc.labels, Running: true}}
					daemon.mu.Unlock()
					registration := environment
					if environment == "0" {
						registration = hub.services.store.RegistrationID
					}
					if tc.manual {
						require.NoError(t, hub.services.db.Create(&origin.Override{RegistrationID: registration, ContainerName: "app", DockerID: runtimeSecondContainerID, Origin: "standalone", AssignedBy: "operator"}).Error)
					}
					hub.env.ReadSnapshots().Put(hoststate.Key{HostID: environment, List: "/containers?"}, []byte(`{"success":true,"data":[{"id":"`+id+`","origin":"standalone","originSource":"manual"}]}`))
					status, body := activationCall(t, hub, "/api/environments/"+environment+"/containers/"+id+"/recreate", uuid.NewString(), "respond-async", []byte(`{"edit":{}}`))
					require.Equal(t, 409, status, string(body))
					require.Contains(t, string(body), "container.origin-locked")
					require.Empty(t, daemon.counts())
					if tc.manual {
						require.NoError(t, hub.services.db.Where("registration_id=?", registration).Delete(&origin.Override{}).Error)
					}
				})
			}
		})
	}
}
