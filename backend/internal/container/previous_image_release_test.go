// Source: docklet-next/issues/409
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original previous-image slot release on container delete.

package container

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// #409.1: deleting the updated container releases its slot and pin, so the
// previous image is no longer reported as protected.
func TestDeleteContainerReleasesPreviousImage(t *testing.T) {
	s, d, p, db := updateFixture(t)
	s.eventService = event.NewEventService(db, &config.Config{}, nil)
	require.NoError(t, s.ExecuteContainerImage(updateContext(t), updateOldID, "app", false, user.Actor{}))
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Equal(t, updateNewID, slot.OwnerContainerID)
	reasons, err := p.g.Reasons(t.Context(), updateOldImage)
	require.NoError(t, err)
	require.NotEmpty(t, reasons)

	require.NoError(t, s.DeleteContainer(t.Context(), "app", true, false, user.Actor{}))
	require.NotContains(t, d.containers, updateNewID)
	slot, err = s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.Nil(t, slot)
	reasons, err = p.g.Reasons(t.Context(), updateOldImage)
	require.NoError(t, err)
	require.Empty(t, reasons)
}

// Another owner's slot under the same name is not released by this delete.
func TestDeleteContainerKeepsForeignOwnerSlot(t *testing.T) {
	s, _, p, db := updateFixture(t)
	s.eventService = event.NewEventService(db, &config.Config{}, nil)
	require.NoError(t, s.replacePreviousImage(t.Context(), "app", updateOtherImage, "registry.example.com/app:older", updateNewID, "older-run"))
	require.NoError(t, s.DeleteContainer(t.Context(), updateOldID, true, false, user.Actor{}))
	slot, err := s.previousImage(t.Context(), "app")
	require.NoError(t, err)
	require.NotNil(t, slot)
	reasons, err := p.g.Reasons(t.Context(), updateOtherImage)
	require.NoError(t, err)
	require.NotEmpty(t, reasons)
}
