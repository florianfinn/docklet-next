// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package handlers

import (
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	userDTO "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// A user assigned on the hub host holds activities:read there, so the channel
// restriction, not the permission checks, keeps foreign activity off the stream.
func TestUserStreamReceivesOnlyEnvironmentsChannel(t *testing.T) {
	input := &StreamClientInput{Channels: "environments,activities,events,dashboard,version"}
	h := &StreamHandler{}
	user := access.Synthesize("viewer", []userDTO.AppAssignment{{EnvironmentID: "0", ProjectID: "project", Level: "full"}})
	require.True(t, user.Allows(authz.PermActivitiesRead, "0"))
	require.Len(t, h.producersForInternal(user, input), 1)

	admin := authz.SudoPermissionSet()
	require.Greater(t, len(h.producersForInternal(admin, input)), 1)
}
