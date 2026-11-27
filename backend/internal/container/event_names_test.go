// Source: docklet-next/issues/679
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original event and sort regression proofs.

package container

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	stats "github.com/florianfinn/docklet-next/backend/internal/container/children/stats"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/florianfinn/docklet-next/types/user"
	docker "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestContainerEventsCarryRealNames(t *testing.T) {
	for _, action := range []string{"lifecycle", "delete", "create"} {
		t.Run(action, func(t *testing.T) {
			s, _, _, db := updateFixture(t)
			s.eventService = event.NewEventService(db, &config.Config{}, nil)
			switch action {
			case "lifecycle":
				require.NoError(t, s.runContainerLifecycleActionInternal(t.Context(), updateOldID, user.Actor{}, containerLifecycleActionInternal{action: "start", eventType: event.EventTypeContainerStart, runContainerAction: func(*client.Client) error { return nil }}))
			case "delete":
				require.NoError(t, s.DeleteContainer(t.Context(), updateOldID, true, false, user.Actor{}))
			case "create":
				_, err := s.CreateContainer(t.Context(), &docker.Config{Image: "registry.example.com/app:1.0.0"}, &docker.HostConfig{}, nil, "app", user.Actor{}, nil)
				require.NoError(t, err)
			}
			var rows []event.Event
			require.NoError(t, db.WithContext(context.Background()).Find(&rows).Error)
			require.NotEmpty(t, rows)
			found := false
			for _, row := range rows {
				if row.ResourceName != nil && *row.ResourceName == "app" {
					found = true
				}
			}
			require.True(t, found, "event payload must contain the resource name")
		})
	}
}
func TestBothContainerNameSortsIgnoreCase(t *testing.T) {
	a, b := dto.Summary{Names: []string{"/alpha"}}, dto.Summary{Names: []string{"/Bravo"}}
	require.Negative(t, stats.CompareContainerNamesForSort(a, b))
	svc := &ContainerService{}
	for _, binding := range svc.buildContainerSortBindings() {
		if binding.Key == "name" {
			require.Negative(t, binding.Fn(a, b))
			return
		}
	}
	t.Fatal("name binding missing")
}
