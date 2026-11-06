// Source: docklet-next/issues/308
// Revision: 8696c990fba68be0dd3ca9ac4de61434f6c56131
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original readable notice and link base tests.

package notification

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/stretchr/testify/require"
)

// Payloads follow the real producers (runtime notifier, backup notifier, availability, Auto Heal store,
// history projection); wire codes may appear only in webhook variables, never in the text.
var m19OutboxCases = []struct {
	event, title, second string
	payload              database.JSON
	codes                []string
}{
	{"deploy_failed", "Update failed: stack web on Example", "Reason: the deployment failed", database.JSON{"host": "Example", "stack": "web", "operation": "update-project", "reason": "runtime.failed"}, []string{"update-project", "runtime.failed"}},
	{"deploy_failed", "Deployment failed: stack web on Example", "Reason: the run was interrupted", database.JSON{"host": "Example", "stack": "web", "operation": "deploy-project", "reason": "runtime.interrupted"}, []string{"deploy-project", "runtime.interrupted"}},
	{"backup_failed", "Backup failed: volume data on Example", "Reason: the backup target is unreachable", database.JSON{"host": "Example", "operation": "backup", "volume": "data", "reason": "backup.target.unreachable", "activityId": "example"}, []string{"backup.target.unreachable"}},
	{"backup_failed", "Restore failed: volume data on Example", "Reason: an unexpected error occurred", database.JSON{"host": "Example", "operation": "restore", "volume": "data", "reason": "volume.example-unknown"}, []string{"volume.example-unknown"}},
	{"host_offline", "Host Example is offline", "", database.JSON{"host": "Example", "outcome": "offline"}, nil},
	{"host_offline", "Host Example is back online", "", database.JSON{"host": "Example", "outcome": "online"}, nil},
	{"auto_heal", "Auto Heal restarted app on Example", "", database.JSON{"container": "app", "stack": "web", "outcome": "restarted", "restarts": 1, "windowMinutes": 60}, nil},
	{"auto_heal", "Auto Heal could not restart app on Example", "", database.JSON{"container": "app", "outcome": "restart-failed", "restarts": 2, "windowMinutes": 60}, []string{"restart-failed"}},
	{"auto_heal", "Auto Heal restart limit reached for app on Example", "3 restarts in 60 min; healing is paused for this container", database.JSON{"container": "app", "outcome": "limit-reached", "restarts": 3, "windowMinutes": 60}, []string{"limit-reached"}},
	{"sustained_load", "High CPU load on Example: 95% above 90% for 10 min", "", database.JSON{"metric": "cpu", "percent": 95.4, "threshold": 90.0, "minutes": 10, "host": "Example", "container": ""}, []string{"cpu"}},
	{"container_unhealthy", "Container app on Example is unhealthy", "Healthcheck failing for 12 min", database.JSON{"host": "Example", "container": "app", "stack": "web", "reason": "healthcheck", "minutes": int64(12), "restarts": 0}, []string{"healthcheck"}},
	{"container_unhealthy", "Container app on Example keeps restarting", "4 restarts in 10 min", database.JSON{"host": "Example", "container": "app", "reason": "restart-loop", "minutes": int64(10), "restarts": 4}, []string{"restart-loop"}},
}

var m19OutboxEventKeys = []string{"deploy_failed", "backup_failed", "host_offline", "auto_heal", "sustained_load", "container_unhealthy"}

func TestM19OutboxNoticesAreReadable(t *testing.T) {
	for _, c := range m19OutboxCases {
		text := outboxText(c.event, "Example", c.payload, "https://docklet.example.com")
		lines := strings.Split(text, "\n")
		require.Equal(t, c.title, lines[0], c.event)
		if c.second != "" {
			require.Equal(t, c.second, lines[1], c.event)
		}
		require.Equal(t, "Open Docklet: https://docklet.example.com"+outboxLinkPaths[c.event], lines[len(lines)-1])
		for _, raw := range append(append([]string{}, m19OutboxEventKeys...), c.codes...) {
			require.NotContains(t, text, raw, c.event)
		}
	}
	for code := range reasonPhrases {
		require.NotEqual(t, unknownReasonPhrase, reasonPhrase(code))
	}
}

// Every channel receives the same readable title and text; the generic webhook keeps the codes as variables.
func TestM19OutboxDeliveryUsesReadableTitleAndKeepsCodes(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	t.Setenv("APP_URL", "https://docklet.example.com")
	s.config = &config.Config{AppUrl: "https://docklet.example.com"}
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}, &notificationoutbox.Availability{}))
	bodies := make(chan string, 2)
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		bodies <- string(b)
	}))
	defer sink.Close()
	require.NoError(t, db.Create(&NotificationSettings{Name: "plain", Provider: "generic", Enabled: true, Config: database.JSON{"webhookUrl": sink.URL}}).Error)
	require.NoError(t, db.Create(&NotificationSettings{Name: "parsed", Provider: "generic", Enabled: true, Config: database.JSON{"webhookUrl": sink.URL, "payloadTemplate": `{"event":"{{.event}}","reason":"{{.reason}}","title":"{{.title}}"}`}}).Error)
	now := time.Now().UTC()
	require.NoError(t, notificationoutbox.Insert(db.DB, notificationoutbox.Row{ID: "deploy", Event: "deploy_failed", EnvironmentID: "0", DedupKey: "deploy", Payload: database.JSON{"host": "Example", "stack": "web", "operation": "deploy-project", "reason": "runtime.failed"}, CreatedAt: now}))
	require.NoError(t, s.DeliverPending(t.Context(), now))
	got := map[string]map[string]string{}
	for range 2 {
		var body map[string]string
		require.NoError(t, json.Unmarshal([]byte(<-bodies), &body))
		if _, ok := body["reason"]; ok {
			got["parsed"] = body
		} else {
			got["plain"] = body
		}
	}
	require.Equal(t, "Deployment failed: stack web on Example", got["plain"]["title"])
	require.Equal(t, "Deployment failed: stack web on Example\nReason: the deployment failed\nOpen Docklet: https://docklet.example.com/stacks", got["plain"]["message"])
	require.Equal(t, map[string]string{"event": "deploy_failed", "reason": "runtime.failed", "title": "Deployment failed: stack web on Example"}, got["parsed"])
}

type m19LinkSettings map[string]string

func (m m19LinkSettings) GetStringSetting(_ context.Context, key, defaultValue string) string {
	if v, ok := m[key]; ok {
		return v
	}
	return defaultValue
}

// Same cases as the web notificationBaseUrl rule, plus the APP_URL fallback without an internal address.
func TestM19LinkBaseFollowsTheLinkSetting(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	t.Setenv("APP_URL", "https://app.example.com/")
	s.config = &config.Config{AppUrl: "https://app.example.com/"}
	require.Equal(t, "https://app.example.com", s.LinkBase(t.Context()), "no settings source")
	for _, c := range []struct {
		settings m19LinkSettings
		want     string
	}{
		{m19LinkSettings{"hubAddress": "192.0.2.10:8080"}, "http://192.0.2.10:8080"},
		{m19LinkSettings{"hubAddress": "192.0.2.10:8080", "domainUrl": "https://docklet.example.com/"}, "https://docklet.example.com"},
		{m19LinkSettings{"hubAddress": "192.0.2.10:8080", "domainUrl": "https://docklet.example.com", "linkBase": "internal"}, "http://192.0.2.10:8080"},
		{m19LinkSettings{"hubAddress": "https://hub.example.com//", "linkBase": "domain"}, "https://hub.example.com"},
		{m19LinkSettings{"hubAddress": " ", "domainUrl": "", "linkBase": "auto"}, "https://app.example.com"},
		{m19LinkSettings{"domainUrl": "https://docklet.example.com", "linkBase": "internal"}, "https://app.example.com"},
	} {
		s.UseLinkSettings(c.settings)
		require.Equal(t, c.want, s.LinkBase(t.Context()), c.settings)
	}
}

// Email templates (AppURL and LogoURL) use the same link base as outbox text.
func TestM19EmailTemplatesUseTheLinkBase(t *testing.T) {
	t.Setenv("APP_URL", "https://app.example.com")
	s := NewNotificationService(nil, &config.Config{AppUrl: "https://app.example.com"}, nil, nil)
	s.UseLinkSettings(m19LinkSettings{"domainUrl": "https://docklet.example.com/"})
	_, html, err := s.templates.TestEmail("Example").RenderEmail()
	require.NoError(t, err)
	require.Contains(t, html, "https://docklet.example.com")
	require.NotContains(t, html, "https://app.example.com")
}
