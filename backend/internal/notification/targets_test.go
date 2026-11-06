// Source: docklet-next/issues/351
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original named-target isolation and contract regression guards.

package notification

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/notifications"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func targetInput(name string, c database.JSON) dto.Update {
	return dto.Update{Name: name, Provider: "ntfy", Enabled: true, Config: base.JsonObject(c)}
}
func TestM19TargetNamesAndCodes(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	h := &NotificationHandler{notificationService: s}
	save := func(name string) (*CreateOrUpdateNotificationSettingsOutput, error) {
		return h.CreateOrUpdateNotificationSettings(t.Context(), &CreateOrUpdateNotificationSettingsInput{EnvironmentID: "0", Body: targetInput(name, database.JSON{"topic": "example"})})
	}
	a, err := save("  Main  ")
	require.NoError(t, err)
	require.Equal(t, "Main", a.Body.Name)
	for _, name := range []string{"", "   ", strings.Repeat("ü", 65), "main"} {
		_, err := save(name)
		var p *dto.Problem
		require.ErrorAs(t, err, &p)
		code, status := "notification.name-invalid", 400
		if name == "main" {
			code, status = "notification.name-taken", 409
		}
		require.Equal(t, code, p.Code)
		require.Equal(t, status, p.Status)
		require.Equal(t, "/name", p.Field)
	}
	_, err = save(strings.Repeat("ü", 64))
	require.NoError(t, err)
	_, err = h.GetNotificationSettings(t.Context(), &GetNotificationSettingsInput{EnvironmentID: "0", TargetID: 999})
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "notification.target-not-found", p.Code)
	err = s.DeleteSettings(t.Context(), 999)
	require.ErrorAs(t, err, &p)
	require.Equal(t, "notification.target-not-found", p.Code)
	_, err = h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", TargetID: 999})
	require.ErrorAs(t, err, &p)
	require.Equal(t, "notification.target-not-found", p.Code)
}
func TestM19SameProviderTargetIsolation(t *testing.T) {
	db, s := setupNotificationTestServiceInternal(t)
	a, err := s.SaveTarget(t.Context(), 0, targetInput("A", database.JSON{"host": "notify.example.com", "topic": "a", "username": "a", "password": strings.Repeat("a", 12), "events": map[string]any{"image_update": false}}))
	require.NoError(t, err)
	b, err := s.SaveTarget(t.Context(), 0, targetInput("B", database.JSON{"host": "notify.example.com", "topic": "b", "username": "b", "password": strings.Repeat("b", 12), "events": map[string]any{"image_update": true}}))
	require.NoError(t, err)
	input := targetInput("Renamed", database.JSON{"host": "notify.example.com", "topic": "b-edited", "username": "b", "password": ""})
	b, err = s.SaveTarget(t.Context(), b.ID, input)
	require.NoError(t, err)
	retained, err := s.GetSettingsByID(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, a.Config, retained.Config)
	require.Equal(t, "A", retained.Name)
	require.NotEqual(t, a.Config["password"], b.Config["password"])
	metadata := database.JSON{"example": true}
	delivered, err := s.notifyEnabledProvidersInternal(t.Context(), NotificationTarget{EnvironmentID: "0"}, "image_update", "example", metadata, func(_ context.Context, _ notifications.NotificationProvider, c database.JSON) (bool, error) {
		require.Equal(t, "b-edited", c["topic"])
		return true, nil
	})
	require.NoError(t, err)
	require.Equal(t, 1, delivered)
	require.NotContains(t, metadata, "targetId")
	var events []event.Event
	require.NoError(t, db.Find(&events).Error)
	require.Len(t, events, 1)
	require.EqualValues(t, b.ID, events[0].Metadata["targetId"])
	require.Equal(t, "Renamed", events[0].Metadata["targetName"])
	immutable := input
	immutable.Provider = "generic"
	_, err = s.SaveTarget(t.Context(), b.ID, immutable)
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "/provider", p.Field)
	require.NoError(t, s.DeleteSettings(t.Context(), b.ID))
	retained, err = s.GetSettingsByID(t.Context(), a.ID)
	require.NoError(t, err)
	require.Equal(t, a.Config, retained.Config)
}
func TestM19TargetProbeLockAndLookupByID(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	entered := make(chan string, 2)
	release := make(chan struct{})
	sink := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { entered <- r.URL.Path; <-release; w.WriteHeader(200) }))
	defer sink.Close()
	defer func() {
		select {
		case <-release:
		default:
			close(release)
		}
	}()
	create := func(name, path string) *NotificationSettings {
		row, err := s.SaveTarget(t.Context(), 0, dto.Update{Name: name, Provider: "generic", Enabled: true, Config: base.JsonObject{"webhookUrl": sink.URL + path}})
		require.NoError(t, err)
		return row
	}
	a, b := create("A", "/a"), create("B", "/b")
	h := &NotificationHandler{notificationService: s}
	done := make(chan error, 2)
	probe := func(id uint) {
		_, err := h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", TargetID: id})
		done <- err
	}
	go probe(a.ID)
	require.Equal(t, "/a", <-entered)
	_, err := h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: "0", TargetID: a.ID})
	var p *dto.Problem
	require.ErrorAs(t, err, &p)
	require.Equal(t, "notification.test-busy", p.Code)
	go probe(b.ID)
	select {
	case path := <-entered:
		require.Equal(t, "/b", path)
	case err := <-done:
		require.NoError(t, err, "second target must enter its own receiver")
		t.Fatal("probe returned without receiver")
	}
	close(release)
	require.NoError(t, <-done)
	require.NoError(t, <-done)
}
func TestM19TargetRoutesHubOnly(t *testing.T) {
	_, s := setupNotificationTestServiceInternal(t)
	for _, cfg := range []*config.Config{{}, {AgentMode: true}, {EdgeAgent: true}} {
		h := &NotificationHandler{notificationService: s, config: cfg}
		ids := []string{"remote"}
		if cfg.AgentMode || cfg.EdgeAgent {
			ids = append(ids, "0")
		}
		for _, id := range ids {
			_, err := h.CreateOrUpdateNotificationSettings(t.Context(), &CreateOrUpdateNotificationSettingsInput{EnvironmentID: id})
			var p *dto.Problem
			require.ErrorAs(t, err, &p)
			require.Equal(t, "notification.hub-only", p.Code)
			_, err = h.UpdateNotificationSettings(t.Context(), &UpdateNotificationSettingsInput{EnvironmentID: id, TargetID: 1})
			require.ErrorAs(t, err, &p)
			require.Equal(t, "notification.hub-only", p.Code)
			_, err = h.DeleteNotificationSettings(t.Context(), &DeleteNotificationSettingsInput{EnvironmentID: id, TargetID: 1})
			require.ErrorAs(t, err, &p)
			require.Equal(t, "notification.hub-only", p.Code)
			_, err = h.TestNotification(t.Context(), &TestNotificationInput{EnvironmentID: id, TargetID: 1})
			require.ErrorAs(t, err, &p)
			require.Equal(t, "notification.hub-only", p.Code)
		}
	}
}
func TestM19UnhealthyNoticeOptIn(t *testing.T) {
	s := &NotificationService{}
	require.False(t, s.isEventEnabled(database.JSON{}, "container_unhealthy"))
	require.True(t, s.isEventEnabled(database.JSON{"events": map[string]any{"container_unhealthy": true}}, "container_unhealthy"))
	require.False(t, s.isEventEnabled(database.JSON{"events": map[string]any{"container_unhealthy": false}}, "container_unhealthy"))
}
