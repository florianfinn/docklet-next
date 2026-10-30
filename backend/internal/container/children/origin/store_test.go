// Source: docklet-next/issues/314
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original origin detection, identity, retention and race guards.

package origin

import (
	"context"
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func fixture(t *testing.T) *Store {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "origins.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	return &Store{DB: db.DB, LocalRegistration: "hub"}
}
func TestDetectOriginPresenceAndPrecedence(t *testing.T) {
	for _, tc := range []struct {
		name   string
		labels map[string]string
		want   dto.Origin
	}{
		{"empty-unraid", map[string]string{"net.unraid.docker.managed": ""}, dto.OriginUnraid},
		{"compose-wins", map[string]string{"net.unraid.docker.managed": "", "com.docker.compose.project": "stack", "com.docker.compose.service": "app"}, dto.OriginCompose},
		{"partial-project", map[string]string{"com.docker.compose.project": ""}, dto.OriginUnknown},
		{"partial-service", map[string]string{"com.docker.compose.service": "app"}, dto.OriginUnknown},
		{"plain", nil, dto.OriginStandalone},
	} {
		t.Run(tc.name, func(t *testing.T) {
			require.Equal(t, dto.OriginFields{Origin: tc.want, OriginSource: dto.OriginDetected}, Detect(tc.labels))
		})
	}
}
func TestOriginOverlayIdentityAndConditionalMove(t *testing.T) {
	s := fixture(t)
	id := Identity{ID: "old", Name: "app", Labels: map[string]string{"com.docker.compose.service": "app"}}
	require.NoError(t, s.Assign(t.Context(), "0", id, dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: id.ID}, "operator"))
	fields, err := s.Effective(t.Context(), "hub", id)
	require.NoError(t, err)
	require.Equal(t, dto.OriginManual, fields.OriginSource)
	changed := id
	changed.ID = "external"
	fields, err = s.Effective(t.Context(), "hub", changed)
	require.NoError(t, err)
	require.Equal(t, dto.OriginUnknown, fields.Origin)
	require.Equal(t, dto.OriginUnconfirmed, fields.OriginSource)
	require.NoError(t, Move(s.DB, "hub", &dto.RecreateResult{OldID: "old", OldName: "app", NewID: "new", NewName: "renamed"}))
	id.ID, id.Name = "new", "renamed"
	fields, err = s.Effective(t.Context(), "hub", id)
	require.NoError(t, err)
	require.Equal(t, dto.OriginManual, fields.OriginSource)
	require.NoError(t, s.Reset(t.Context(), "0", id))
	require.NoError(t, Move(s.DB, "hub", &dto.RecreateResult{OldID: "new", OldName: "renamed", NewID: "late", NewName: "late"}))
	var n int64
	require.NoError(t, s.DB.Model(&Override{}).Count(&n).Error)
	require.Zero(t, n)
}
func TestOriginPartialProjectStandaloneRefused(t *testing.T) {
	s := fixture(t)
	id := Identity{ID: "example", Name: "app", Labels: map[string]string{"com.docker.compose.project": ""}}
	require.Error(t, s.Assign(t.Context(), "0", id, dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: id.ID}, "operator"))
	require.NoError(t, s.Assign(t.Context(), "0", id, dto.OriginAssignment{Origin: dto.OriginUnraid, DockerID: id.ID}, "operator"))
	require.Error(t, s.Guard(t.Context(), "hub", id))
}
func TestOriginConcurrentResetFencesAssignment(t *testing.T) {
	s := fixture(t)
	id := Identity{ID: "old", Name: "app", Labels: map[string]string{"com.docker.compose.service": "app"}}
	request := dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: "old"}
	require.NoError(t, s.Assign(t.Context(), "0", id, request, "operator"))
	reading, release := make(chan struct{}), make(chan struct{})
	done := make(chan error, 1)
	go func() {
		done <- s.AssignFresh(t.Context(), "0", "old", request, "another", func(context.Context, string, string) (Identity, error) { close(reading); <-release; return id, nil })
	}()
	select {
	case <-reading:
	case <-time.After(5 * time.Second):
		t.Fatal("assignment never reached host read")
	}
	require.NoError(t, s.Reset(t.Context(), "0", id))
	close(release)
	require.Error(t, <-done, "a stale assignment cannot resurrect a reset")
	var rows []Override
	require.NoError(t, s.DB.Find(&rows).Error)
	require.Empty(t, rows)
}
func TestOriginOverlayListGroupAndDetail(t *testing.T) {
	s := fixture(t)
	id := Identity{ID: "old", Name: "app", Labels: map[string]string{"com.docker.compose.service": "app"}}
	require.NoError(t, s.Assign(t.Context(), "0", id, dto.OriginAssignment{Origin: dto.OriginStandalone, DockerID: "old"}, "operator"))
	for _, body := range []string{`{"success":true,"data":[{"id":"external","names":["/app"],"origin":"unknown","labels":{"com.docker.compose.service":"app"}}]}`, `{"success":true,"data":{"id":"external","name":"app","origin":"unknown","labels":{"com.docker.compose.service":"app"}}}`} {
		out, err := s.Overlay(t.Context(), "0", []byte(body))
		require.NoError(t, err)
		var value map[string]any
		require.NoError(t, json.Unmarshal(out, &value))
		require.Contains(t, string(out), `"originSource":"unconfirmed"`)
		require.Contains(t, string(out), `"origin":"unknown"`)
	}
}
