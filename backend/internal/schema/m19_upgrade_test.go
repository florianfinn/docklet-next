// Source: docklet-next/issues/351
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original M18 upgrade, delivery and immutable-state preservation evidence.

package schema

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/notification/notificationoutbox"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"testing/fstest"
	"time"
)

func TestM19Migration019BothRolesOldDataAndDelivery(t *testing.T) {
	for _, role := range []string{"hub", "agent"} {
		for _, empty := range []bool{false, true} {
			t.Run(role+map[bool]string{false: "/old", true: "/empty"}[empty], func(t *testing.T) {
				path := filepath.Join(t.TempDir(), "upgrade.db")
				pool, err := sqlitestore.Open(t.Context(), path, true, false)
				require.NoError(t, err)
				pool.SetMaxOpenConns(1)
				defer pool.Close()
				g, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{})
				require.NoError(t, err)
				names, err := filepath.Glob("../../resources/migrations/sqlite/*.sql")
				require.NoError(t, err)
				fs := fstest.MapFS{}
				for _, name := range names {
					data, err := os.ReadFile(name)
					require.NoError(t, err)
					fs[filepath.Base(name)] = &fstest.MapFile{Data: data}
				}
				provider, err := goose.NewProvider(goose.DialectSQLite3, pool, fs)
				require.NoError(t, err)
				_, err = provider.UpTo(t.Context(), 18)
				require.NoError(t, err)
				calls := make(chan struct{}, 2)
				sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls <- struct{}{}; w.WriteHeader(200) }))
				defer sink.Close()
				configJSON, err := json.Marshal(map[string]any{"webhookUrl": sink.URL, "events": map[string]bool{"backup_failed": true}})
				require.NoError(t, err)
				if !empty {
					for i, p := range []string{"generic", "email", "discord", "telegram", "signal", "slack", "ntfy", "pushover", "gotify", "matrix", "googlechat", "ntfy", "ntfy"} {
						require.NoError(t, g.Exec("INSERT INTO notification_settings(id,provider,enabled,config,created_at) VALUES(?,?,1,?,?)", i+1, p, string(configJSON), time.Date(2026, 11, 5, 0, 0, i, 0, time.UTC)).Error)
					}
				}
				now := time.Now().UTC().Truncate(time.Second)
				statuses := []string{"pending", "sending", "sent", "failed", "unconfigured", "expired"}
				for i, status := range statuses {
					var lease, settled any
					if status == "sending" {
						lease = now.Add(time.Hour)
					}
					if i > 1 {
						settled = now
					}
					require.NoError(t, g.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,attempts,lease_until,created_at,settled_at) VALUES(?,?,?,?,?,?,?,?,?,?)", status, "backup_failed", "0", "dedup:"+status, `{"host":"Example"}`, status, i, lease, now, settled).Error)
				}
				var before []notificationoutbox.Row
				require.NoError(t, g.Order("id").Find(&before).Error)
				require.NoError(t, pool.Close())
				initialize := database.Initialize
				if role == "hub" {
					initialize = database.InitializeHub
				}
				upgraded, err := initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
				require.NoError(t, err)
				defer upgraded.Close()
				g = upgraded.DB
				var after []notificationoutbox.Row
				require.NoError(t, g.Order("id").Find(&after).Error)
				require.Equal(t, before, after, "every status/lease/attempt/dedup value is preserved")
				require.NoError(t, g.Exec("INSERT INTO notification_outbox(id,event,environment_id,dedup_key,payload,status,attempts,created_at) VALUES('unhealthy','container_unhealthy','0','unhealthy','{}','pending',0,?)", now).Error)
				require.Error(t, g.Exec("UPDATE notification_outbox SET event='unknown' WHERE id='unhealthy'").Error)
				require.Error(t, g.Exec("UPDATE notification_outbox SET dedup_key='dedup:pending' WHERE id='unhealthy'").Error)
				var count int64
				require.NoError(t, g.Raw("SELECT count(*) FROM sqlite_master WHERE type='index' AND name='idx_notification_outbox_delivery'").Scan(&count).Error)
				require.EqualValues(t, 1, count)
				require.True(t, g.Migrator().HasTable("history_conditions"))
				if !empty {
					var targets []notification.NotificationSettings
					require.NoError(t, g.Order("id").Find(&targets).Error)
					require.Equal(t, []string{"Generic webhook", "Email", "Discord", "Telegram", "Signal", "Slack", "ntfy", "Pushover", "Gotify", "Matrix", "Google Chat", "ntfy 2", "ntfy 3"}, func() []string {
						r := []string{}
						for _, v := range targets {
							r = append(r, v.Name)
						}
						return r
					}())
					require.JSONEq(t, string(configJSON), func() string { b, _ := json.Marshal(targets[0].Config); return string(b) }())
					require.Error(t, g.Exec("INSERT INTO notification_settings(provider,name) VALUES('generic','EMAIL')").Error)
					if role == "hub" {
						require.NoError(t, g.Model(&notification.NotificationSettings{}).Where("id<>1").Update("enabled", false).Error)
						require.NoError(t, g.Where("id<>?", "pending").Delete(&notificationoutbox.Row{}).Error)
						svc := notification.NewNotificationService(&database.DB{DB: g}, &config.Config{}, nil, nil)
						require.NoError(t, svc.DeliverPending(t.Context(), now))
						select {
						case <-calls:
						default:
							t.Fatal("upgraded target did not receive its subscribed notice")
						}
						var sent notificationoutbox.Row
						require.NoError(t, g.First(&sent, "id=?", "pending").Error)
						require.Equal(t, "sent", sent.Status)
					}
				}
			})
		}
	}
}
