// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bounded producer resource-name regression tests for M41 S3.

package network

import (
	"context"
	"path/filepath"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	networktypes "github.com/florianfinn/docklet-next/types/network"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	"github.com/stretchr/testify/require"
)

func nameNetworkFixture(t *testing.T, operation string) *networkDockerFixture {
	t.Helper()
	f := &networkDockerFixture{networks: []network.Inspect{inspectNetwork()}}
	if operation == "connect" {
		f.containers = []container.InspectResponse{disconnectedContainer()}
	}
	if operation == "disconnect" {
		f.containers = []container.InspectResponse{stoppedReference()}
	}
	return f
}

func TestNetworkEventResourceNames(t *testing.T) {
	for _, operation := range []string{"connect", "disconnect", "remove"} {
		t.Run(operation, func(t *testing.T) {
			f := nameNetworkFixture(t, operation)
			svc := f.service(t)
			switch operation {
			case "connect":
				require.NoError(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1"}, actor))
			case "disconnect":
				require.NoError(t, svc.DisconnectContainer(t.Context(), "net1", networktypes.DisconnectContainerRequest{ContainerID: "stopped"}, actor))
			case "remove":
				require.NoError(t, svc.RemoveNetwork(t.Context(), "net1", actor))
			}
			require.Equal(t, []string{operation}, f.writes)
			var stored event.Event
			require.NoError(t, svc.db.First(&stored).Error)
			require.Equal(t, "net1", *stored.ResourceID, "event resource id must remain unchanged")
			require.NotNil(t, stored.ResourceName)
			require.Equal(t, f.networks[0].Name, *stored.ResourceName, "event resource name must use inspected network name")
		})
	}
}

func TestNetworkActivityResourceNames(t *testing.T) {
	for _, operation := range []string{"connect", "disconnect", "remove"} {
		for _, fallback := range []bool{false, true} {
			label := "name"
			if fallback {
				label = "lookup-failure"
			}
			t.Run(operation+"/"+label, func(t *testing.T) {
				f := nameNetworkFixture(t, operation)
				f.failNameLookup = fallback
				svc := f.service(t)
				db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "hub.db")), database.MigrationOptions{})
				require.NoError(t, err)
				t.Cleanup(func() { require.NoError(t, db.Close()) })
				svc.db, svc.eventService = db, event.NewEventService(db, nil, nil)
				h := &NetworkHandler{networkService: svc, activityService: activity.NewActivityService(db, nil), appCtx: t.Context()}
				ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &actor)
				switch operation {
				case "connect":
					_, err = h.ConnectContainer(ctx, &ConnectContainerInput{EnvironmentID: "0", NetworkID: "net1", Body: networktypes.ConnectContainerRequest{ContainerID: "container1"}})
				case "disconnect":
					_, err = h.DisconnectContainer(ctx, &DisconnectContainerInput{EnvironmentID: "0", NetworkID: "net1", Body: networktypes.DisconnectContainerRequest{ContainerID: "stopped"}})
				case "remove":
					_, err = h.DeleteNetwork(ctx, &DeleteNetworkInput{EnvironmentID: "0", NetworkID: "net1"})
				}
				require.NoError(t, err, "display lookup failure must not refuse the action")
				require.Equal(t, []string{operation}, f.writes)
				var stored activity.Activity
				require.NoError(t, db.First(&stored).Error)
				require.Equal(t, activitytypes.StatusSuccess, stored.Status)
				require.Equal(t, "net1", *stored.ResourceID, "activity resource id must remain unchanged")
				expected := f.networks[0].Name
				if fallback {
					expected = "net1"
				}
				require.NotNil(t, stored.ResourceName)
				require.Equal(t, expected, *stored.ResourceName, "activity resource name must use the name or id fallback")
				require.Equal(t, 2, f.networkReads, "the action must still perform its own validation inspect")
			})
		}
	}
}

func TestNetworkEventResourceNameFallback(t *testing.T) {
	for _, operation := range []string{"connect", "disconnect"} {
		t.Run(operation, func(t *testing.T) {
			f := nameNetworkFixture(t, operation)
			f.networks[0].Name = ""
			svc := f.service(t)
			if operation == "connect" {
				require.NoError(t, svc.ConnectContainer(t.Context(), "net1", networktypes.ConnectContainerRequest{ContainerID: "container1"}, actor))
			} else {
				require.NoError(t, svc.DisconnectContainer(t.Context(), "net1", networktypes.DisconnectContainerRequest{ContainerID: "stopped"}, actor))
			}
			require.Equal(t, []string{operation}, f.writes)
			var stored event.Event
			require.NoError(t, svc.db.First(&stored).Error)
			require.Equal(t, "net1", *stored.ResourceID)
			require.Equal(t, "net1", *stored.ResourceName, "missing display name must retain the id")
		})
	}
}
