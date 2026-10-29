// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestOutboxAttemptsExpiryPruneAndPartialDelivery(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	now := time.Now().UTC()
	var calls atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1); w.WriteHeader(200) }))
	defer sink.Close()
	require.NoError(t, db.Create(&NotificationSettings{Provider: "generic", Enabled: true, Config: database.JSON{"webhookUrl": sink.URL}}).Error)
	require.NoError(t, db.Create(&NotificationSettings{Provider: "gotify", Enabled: true, Config: database.JSON{}}).Error)
	row := notificationoutbox.Row{ID: "one", Event: "backup_failed", EnvironmentID: "0", DedupKey: "one", Payload: database.JSON{}, CreatedAt: now, Status: "pending"}
	require.NoError(t, notificationoutbox.Insert(db.DB, row))
	require.NoError(t, s.DeliverPending(t.Context(), now))
	require.NoError(t, db.First(&row, "id='one'").Error)
	require.Equal(t, "sent", row.Status)
	require.Equal(t, 1, row.Attempts)
	require.EqualValues(t, 1, calls.Load())
	var n int64
	require.NoError(t, db.Model(&event.Event{}).Where("type='notification.delivery-failed'").Count(&n).Error)
	require.EqualValues(t, 1, n)
	require.NoError(t, db.Model(&NotificationSettings{}).Where("provider='generic'").Update("enabled", false).Error)
	r := notificationoutbox.Row{ID: "fail", Event: "backup_failed", EnvironmentID: "0", DedupKey: "fail", Payload: database.JSON{}, Status: "sending", Attempts: 1, LeaseUntil: new(now.Add(-time.Second)), CreatedAt: now}
	require.NoError(t, db.Create(&r).Error)
	for i := 2; i <= 3; i++ {
		require.NoError(t, s.DeliverPending(t.Context(), now))
		require.NoError(t, db.First(&r, "id='fail'").Error)
		require.Equal(t, i, r.Attempts)
	}
	require.Equal(t, "failed", r.Status)
	require.NoError(t, s.DeliverPending(t.Context(), now))
	require.NoError(t, db.First(&r, "id='fail'").Error)
	require.Equal(t, 3, r.Attempts)
	require.NoError(t, notificationoutbox.Insert(db.DB, notificationoutbox.Row{ID: "old", Event: "backup_failed", EnvironmentID: "0", DedupKey: "old", Payload: database.JSON{}, CreatedAt: now.Add(-25 * time.Hour)}))
	require.NoError(t, s.DeliverPending(t.Context(), now))
	var old notificationoutbox.Row
	require.NoError(t, db.First(&old, "id='old'").Error)
	require.Equal(t, "expired", old.Status)
	require.NoError(t, s.DeliverPending(t.Context(), now.Add(31*24*time.Hour)))
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.Zero(t, n)
}
func TestOutboxConcurrentClaimIsCAS(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
	var calls atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls.Add(1) }))
	defer sink.Close()
	require.NoError(t, db.Create(&NotificationSettings{Provider: "generic", Enabled: true, Config: database.JSON{"webhookUrl": sink.URL}}).Error)
	require.NoError(t, notificationoutbox.Insert(db.DB, notificationoutbox.Row{Event: "backup_failed", EnvironmentID: "0", DedupKey: "one", Payload: database.JSON{}}))
	arrived := make(chan struct{}, 2)
	release := make(chan struct{})
	s.beforeClaim = func() { arrived <- struct{}{}; <-release }
	var wg sync.WaitGroup
	errs := make(chan error, 2)
	now := time.Now().UTC()
	for range 2 {
		wg.Go(func() { errs <- s.DeliverPending(t.Context(), now) })
	}
	<-arrived
	<-arrived
	close(release)
	wg.Wait()
	close(errs)
	for e := range errs {
		require.NoError(t, e)
	}
	require.EqualValues(t, 1, calls.Load())
}
func TestRestoreExpiresOnlyUnfinishedNotifications(t *testing.T) {
	db, _ := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	now := time.Now().UTC()
	for _, status := range []string{"pending", "sending", "sent"} {
		r := notificationoutbox.Row{ID: status, Event: "backup_failed", EnvironmentID: "0", DedupKey: status, Payload: database.JSON{}, CreatedAt: now, Status: status}
		if status == "sending" {
			r.LeaseUntil = &now
		}
		require.NoError(t, db.Create(&r).Error)
	}
	require.NoError(t, db.Create(&notificationoutbox.Availability{EnvironmentID: "remote", OfflineSince: now}).Error)
	require.NoError(t, notificationoutbox.ExpireRestored(t.Context(), db))
	var rows []notificationoutbox.Row
	require.NoError(t, db.Order("id").Find(&rows).Error)
	require.Equal(t, []string{"expired", "expired", "sent"}, []string{rows[0].Status, rows[1].Status, rows[2].Status})
	require.Nil(t, rows[1].LeaseUntil)
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Availability{}).Count(&n).Error)
	require.Zero(t, n)
}

var _ = context.Background
var _ = errors.New
