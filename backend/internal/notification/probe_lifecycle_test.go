// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
)

func TestProviderProbeHasOneFlightAndReleasesAfterCompletion(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Int32
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if calls.Add(1) == 1 {
			close(entered)
			<-release
		}
		w.WriteHeader(200)
	}))
	defer sink.Close()
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	defer unblock()
	require.NoError(t, db.Create(&NotificationSettings{Provider: "generic", Config: database.JSON{"webhookUrl": sink.URL}}).Error)
	h := &NotificationHandler{notificationService: s}
	done := make(chan error, 1)
	go func() {
		_, e := h.TestNotification(context.Background(), &TestNotificationInput{EnvironmentID: "0", Provider: "generic"})
		done <- e
	}()
	<-entered
	_, e := h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", Provider: "generic"})
	var p *dto.Problem
	require.ErrorAs(t, e, &p)
	require.Equal(t, "notification.test-busy", p.Code)
	require.EqualValues(t, 1, calls.Load())
	unblock()
	require.NoError(t, <-done)
	_, e = h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", Provider: "generic"})
	require.NoError(t, e)
	require.EqualValues(t, 2, calls.Load())
}
