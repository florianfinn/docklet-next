// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Existing channel fan-out keeps the durable entry ID and neutralises provider text.

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestPolicyNotificationChannelEntryAndNeutralFailure(t *testing.T) {
	db := setupNotificationTestDB(t)
	svc := NewNotificationService(db, &config.Config{}, nil, event.NewEventService(db, nil, nil))
	require.NoError(t, db.Create(&NotificationSettings{Provider: notifications.NotificationProviderGotify, Enabled: true, Config: database.JSON{}}).Error)
	ctx := context.WithValue(t.Context(), imagePolicyEntryKey{}, "example-durable-entry")
	delivered, err := svc.notifyEnabledProvidersInternal(ctx, NotificationTarget{EnvironmentID: "0", EnvironmentName: "Example host"}, notifications.NotificationEventImageUpdate, "registry.example.com/app:1.0.0", database.JSON{"eventType": "image_update"}, func(context.Context, notifications.NotificationProvider, database.JSON) (bool, error) {
		return true, errors.New("synthetic-private-provider-detail")
	})
	require.Error(t, err)
	require.Zero(t, delivered)
	require.NotContains(t, err.Error(), "synthetic-private-provider-detail")
	var events []event.Event
	require.NoError(t, db.Where("type = ?", event.EventTypeNotificationSend).Find(&events).Error)
	require.Len(t, events, 1)
	payload, err := json.Marshal(events[0])
	require.NoError(t, err)
	require.Contains(t, string(payload), "example-durable-entry")
	require.NotContains(t, string(payload), "synthetic-private-provider-detail")
}
