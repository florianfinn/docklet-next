// Source: getarcaneapp/arcane/backend/pkg/libarcane/volumes/volume_rename_followup_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package volumes

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDockerProjectVolumeRenameMigrationInternal_RollbackExplainsPreservedTargetWhenSourceMissing(t *testing.T) {
	var targetRemoved atomic.Bool

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Name": "web_data"})) {
				return
			}
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			targetRemoved.Store(true)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	migration := &dockerProjectVolumeRenameMigrationInternal{
		dockerClient: newTestDockerClientInternal(t, server),
		createdNew: []volumetypes.RenameEntry{
			{OldName: "nginx_data", NewName: "web_data"},
		},
	}

	err := migration.Rollback(t.Context())

	require.Error(t, err)
	var preserved *volumetypes.TargetPreservedDuringRollbackError
	require.ErrorAs(t, err, &preserved)
	require.Contains(t, err.Error(), "avoid data loss")
	require.Contains(t, err.Error(), "only remaining data copy")
	require.False(t, targetRemoved.Load())
}

func TestRollbackVolume_ExplainsPreservedTargetWhenSourceMissing(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/nginx_data"):
			http.NotFound(w, r)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/volumes/web_data"):
			w.Header().Set("Content-Type", "application/json")
			if !assert.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Name": "web_data"})) {
				return
			}
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)

	err := RollbackVolume(t.Context(), newTestDockerClientInternal(t, server), volumetypes.JournalVolume{
		OldName: "nginx_data",
		NewName: "web_data",
	})

	require.Error(t, err)
	var preserved *volumetypes.TargetPreservedDuringRollbackError
	require.ErrorAs(t, err, &preserved)
	require.Contains(t, err.Error(), "avoid data loss")
	require.Contains(t, err.Error(), "only remaining data copy")
}
