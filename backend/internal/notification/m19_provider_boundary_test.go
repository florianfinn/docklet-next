// Source: docklet-next/issues/308
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original provider-boundary tests for readable notices and classified test results.

package notification

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
)

type m19Transport func(*http.Request) (*http.Response, error)

func (f m19Transport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// captureOutgoing replaces the default transport (used by the provider senders) for one test.
func captureOutgoing(t *testing.T, reply string, err error) chan *http.Request {
	t.Helper()
	original := http.DefaultTransport
	t.Cleanup(func() { http.DefaultTransport = original })
	got := make(chan *http.Request, 4)
	http.DefaultTransport = m19Transport(func(r *http.Request) (*http.Response, error) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(strings.NewReader(string(body)))
		got <- r
		if err != nil {
			return nil, err
		}
		return &http.Response{StatusCode: 200, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(reply)), Request: r}, nil
	})
	return got
}

func outgoing(t *testing.T, got chan *http.Request) *http.Request {
	t.Helper()
	select {
	case r := <-got:
		return r
	case <-time.After(5 * time.Second):
		t.Fatal("no outgoing provider request")
		return nil
	}
}

// A default ntfy target gets the same readable title as the generic webhook and email.
func TestM19NtfyOutboxNoticeCarriesTheTitle(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	got := captureOutgoing(t, `{}`, nil)
	require.NoError(t, db.Create(&NotificationSettings{Name: "ntfy", Provider: "ntfy", Enabled: true, Config: database.JSON{"host": "notify.example.com", "topic": "example", "disableTLS": true}}).Error)
	now := time.Now().UTC()
	require.NoError(t, notificationoutbox.Insert(db.DB, notificationoutbox.Row{ID: "deploy", Event: "deploy_failed", EnvironmentID: "0", DedupKey: "deploy", Payload: database.JSON{"host": "Example", "stack": "web", "reason": "runtime.failed"}, CreatedAt: now}))
	require.NoError(t, s.DeliverPending(t.Context(), now))
	require.Equal(t, "Deployment failed: stack web on Example", outgoing(t, got).Header.Get("Title"))
}

// Telegram's HTML parse mode renders literal newlines and rejects <br>; outbox and scan notices keep newlines.
func TestM19TelegramNoticesUseLiteralNewlines(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	got := captureOutgoing(t, `{"ok":true,"result":{"message_id":1}}`, nil)
	// Telegram tokens are "<bot id>:<secret>"; the synthetic one is assembled to keep the format.
	telegram := database.JSON{"botToken": strings.Join([]string{"123", "example"}, ":"), "chatIds": []string{"123"}}
	check := func(want string) {
		var request map[string]any
		body, _ := io.ReadAll(outgoing(t, got).Body)
		require.NoError(t, json.Unmarshal(body, &request))
		require.Equal(t, "HTML", request["parse_mode"])
		require.NotContains(t, request["text"], "<br>")
		require.Contains(t, request["text"], want)
	}
	_, err := notifications.Deliver(context.Background(), "telegram", telegram, s.templates.VulnerabilityScan(dto.DispatchVulnerabilityScan{HostName: "Example", Outcome: "completed"}))
	require.NoError(t, err)
	check("Host: Example\nOutcome: completed")

	require.NoError(t, db.Create(&NotificationSettings{Name: "telegram", Provider: "telegram", Enabled: true, Config: telegram}).Error)
	now := time.Now().UTC()
	require.NoError(t, notificationoutbox.Insert(db.DB, notificationoutbox.Row{ID: "deploy", Event: "deploy_failed", EnvironmentID: "0", DedupKey: "deploy", Payload: database.JSON{"host": "Example", "stack": "web", "reason": "runtime.failed"}, CreatedAt: now}))
	require.NoError(t, s.DeliverPending(t.Context(), now))
	check("Deployment failed: stack web on Example\nReason: the deployment failed")
}

// The test endpoint returns only the classified code; provider text never reaches the response.
func TestM19TestEndpointReturnsOnlyTheClassifiedReason(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	captureOutgoing(t, "", errors.New("HTTP status 401 at https://notify.example.com/credential-marker"))
	row := NotificationSettings{Name: "probe", Provider: "generic", Config: database.JSON{"webhookUrl": "https://notify.example.com"}}
	require.NoError(t, db.Create(&row).Error)
	h := &NotificationHandler{notificationService: s}
	response, err := h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", TargetID: row.ID})
	require.NoError(t, err)
	require.Equal(t, "notification.test-failed", string(response.Body.Data.Code))
	require.Equal(t, "delivery-auth", string(response.Body.Data.Reason))
	body, err := json.Marshal(response)
	require.NoError(t, err)
	require.NotContains(t, string(body), "credential-marker")
}
