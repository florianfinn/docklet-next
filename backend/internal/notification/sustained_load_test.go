// Source: docklet-next/issues/338
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original sustained-load target default compatibility tests.

package notification

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestHB17SustainedLoadIsOptInForEveryTargetShape(t *testing.T) {
	s := &NotificationService{}
	for _, config := range []database.JSON{nil, {}, {"events": map[string]any{}}, {"events": map[string]any{"auto_heal": true}}, {"events": map[string]any{"sustained_load": false}}} {
		require.False(t, s.isEventEnabled(config, notifications.NotificationEventSustainedLoad))
		require.True(t, s.isEventEnabled(config, notifications.NotificationEventHostOffline))
	}
	require.True(t, s.isEventEnabled(database.JSON{"events": map[string]any{"sustained_load": true}}, notifications.NotificationEventSustainedLoad))
}
