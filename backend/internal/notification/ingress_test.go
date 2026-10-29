// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation.

package notification

import (
	"bytes"
	"encoding/json/v2"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/humatest"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"net/http"
	"reflect"
	"strings"
	"testing"
)

func TestDispatchHTTPKindsAndDeduplication(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
	token, key := "example", "key"
	env := environment.Environment{ID: "remote", Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}
	require.NoError(t, db.Create(&env).Error)
	cfg := huma.DefaultConfig("test", "1")
	cfg.Components.Schemas = huma.NewMapRegistry("#/components/schemas/", func(t reflect.Type, h string) string { return t.PkgPath() + "." + huma.DefaultSchemaNamer(t, h) })
	_, api := humatest.New(t, cfg)
	RegisterNotifications(api, s, &config.Config{})
	valid := dto.DispatchRequest{Kind: "backup_failed", BackupFailed: &dto.DispatchBackupFailed{EventID: "run:one", Operation: "backup", Volume: "data", Reason: "volume.backup.failed"}}
	for range 2 {
		b, err := json.Marshal(valid)
		require.NoError(t, err)
		r := api.Post("/notifications/dispatch", "X-API-Key: "+token, "Content-Type: application/json", bytes.NewReader(b))
		require.Equal(t, 200, r.Code, r.Body.String())
	}
	var n int64
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
	for _, body := range []string{`{"kind":"auto_heal","autoHeal":{"containerName":"web","containerId":"old"}}`, `{"kind":"backup_failed","backupFailed":{"eventId":"two","operation":"backup","volume":"data","reason":"volume.backup.failed","extra":true}}`, `{"kind":"deploy_failed"}`, `{"kind":"backup_failed","backupFailed":{"eventId":"bad space","operation":"backup","volume":"data","reason":"volume.backup.failed"}}`} {
		r := api.Post("/notifications/dispatch", "X-API-Key: "+token, "Content-Type: application/json", strings.NewReader(body))
		require.Equal(t, http.StatusBadRequest, r.Code, r.Body.String())
		require.Contains(t, r.Body.String(), "notification.dispatch-")
	}
	// Current legacy producer shape needs no new event identity and never enters this outbox.
	r := api.Post("/notifications/dispatch", "X-API-Key: "+token, "Content-Type: application/json", strings.NewReader(`{"kind":"prune_report","pruneReport":{"result":{}}}`))
	require.Equal(t, 200, r.Code, r.Body.String())
	require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
	require.EqualValues(t, 1, n)
}
func TestDispatchRechecksOriginalBindingInTransaction(t *testing.T) {
	for _, change := range []string{"rotate", "repair", "remove", "edge-reconnect"} {
		t.Run(change, func(t *testing.T) {
			db, s := setupNotificationTestServiceInternal(t)
			require.NoError(t, db.AutoMigrate(&notificationoutbox.Row{}))
			token, key := "example", "key"
			env := environment.Environment{ID: "remote", Name: "Example", Enabled: true, AccessToken: &token, ApiKeyID: &key}
			require.NoError(t, db.Create(&env).Error)
			origin := originFor(&env)
			session := "old"
			s.SetDispatchSessionCurrent(func(o dto.DispatchOrigin) bool { return o.SessionID == session })
			entered, release := make(chan struct{}), make(chan struct{})
			s.dispatchBarrier = func() { close(entered); <-release }
			p := dto.DispatchRequest{Kind: "auto_heal", AutoHeal: &dto.DispatchAutoHeal{EventID: "one", Outcome: "restarted", Container: "web", Restarts: 1, WindowMinutes: 30}}
			if change == "edge-reconnect" {
				origin.SessionID = "old"
			}

			done := make(chan error, 1)
			go func() { _, err := s.DispatchIngress(t.Context(), origin, p); done <- err }()
			<-entered
			switch change {
			case "edge-reconnect":
				session = "replacement"
			case "rotate":
				require.NoError(t, db.Model(&env).Update("access_token", "changeme").Error)
			case "repair":
				require.NoError(t, db.Model(&env).Update("api_key_id", "replacement").Error)
			case "remove":
				require.NoError(t, db.Delete(&env).Error)
			}
			close(release)
			err := <-done
			var problem *dto.Problem
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "notification.dispatch-stale", problem.Code)
			var n int64
			require.NoError(t, db.Model(&notificationoutbox.Row{}).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}
func TestDispatchOriginMatchesHash(t *testing.T) {
	token := "example"
	require.Equal(t, edge.AgentTokenHash(token), originFor(&environment.Environment{AccessToken: &token}).TokenHash)
}
