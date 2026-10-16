// Source: florianfinn/docklet-next/backend/internal/event
// Revision: 65c9e0dceb6f994ac884904e2a6b7348f75580fa
// Retrieved: 2026-10-14
// License: Apache-2.0
// Deviations: New PostgreSQL event count regression.

package event

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestEventRetentionKeepsNewest500(t *testing.T) {
	db := setupEventServiceTestDB(t)
	svc := NewEventService(db, nil, nil)
	var oldest string
	for i := range 501 {
		created, err := svc.CreateEvent(t.Context(), CreateEventRequest{Type: EventTypeProjectDeploy, Title: fmt.Sprintf("event-%d", i)})
		require.NoError(t, err)
		if i == 0 {
			oldest = created.ID
		}
	}
	require.NoError(t, db.Model(&Event{}).Where("id = ?", oldest).Update("timestamp", time.Now().Add(-time.Hour)).Error)
	deleted, err := svc.PruneEvents(t.Context(), 500)
	require.NoError(t, err)
	require.EqualValues(t, 1, deleted)
	var count int64
	require.NoError(t, db.Model(&Event{}).Count(&count).Error)
	require.EqualValues(t, 500, count)
	require.NoError(t, db.Model(&Event{}).Where("id = ?", oldest).Count(&count).Error)
	require.Zero(t, count)
}
