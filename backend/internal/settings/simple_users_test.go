// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package settings

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestUserSettingsReadContainsOnlySetupStatus(t *testing.T) {
	require.Equal(t, access.Self, access.Rules["get-settings"].Kind)
	svc, err := newSettingsServiceForTestInternal(t, t.Context(), setupSettingsTestDB(t))
	require.NoError(t, err)
	h := &SettingsHandler{settingsService: svc}
	ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, access.Synthesize("viewer", nil))
	out, err := h.GetSettings(ctx, &GetSettingsInput{EnvironmentID: "0"})
	require.NoError(t, err)
	require.Len(t, out.Body, 1)
	require.Equal(t, "setupCompleted", out.Body[0].Key)
}
