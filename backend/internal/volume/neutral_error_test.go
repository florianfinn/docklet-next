// Source: docklet-next/issues/227
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestVolumeCreateRenameNeverExposeDaemonDetails(t *testing.T) {
	for _, route := range []string{"/volumes", "/volumes/app/rename"} {
		t.Run(route, func(t *testing.T) {
			diagnostic := "synthetic daemon diagnostic"
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(500)
				_ = json.NewEncoder(w).Encode(map[string]string{"message": diagnostic})
			}))
			defer server.Close()
			db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, db.AutoMigrate(&event.Event{}))
			pool, err := db.DB()
			require.NoError(t, err)
			defer pool.Close()
			wrapped := &database.DB{DB: db}
			svc := NewVolumeService(wrapped, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), event.NewEventService(wrapped, nil, nil), nil, nil, nil, nil, nil, nil, nil)
			call := pruneTransport(t, "direct", bindTestRouter(t, svc))
			status, body := call("POST", "/api/environments/0"+route, "", []byte(`{"name":"new"}`))
			require.Equal(t, 500, status, string(body))
			require.Contains(t, string(body), "volume.operation-failed")
			require.NotContains(t, string(body), diagnostic)
			err = neutralVolumeOperationError(errors.New(diagnostic))
			require.EqualError(t, err, "volume.operation-failed", "activity callback receives only the neutral error")
		})
	}
}

func TestVolumeInternalHTTPFailureIsNeutral(t *testing.T) {
	require.EqualError(t, neutralVolumeOperationError(huma.Error500InternalServerError("synthetic daemon diagnostic")), "volume.operation-failed")
}
