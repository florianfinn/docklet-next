// Source: getarcaneapp/arcane/backend/internal/activity/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-14
// License: BSD-3-Clause
// Deviations: SQLite persistence test split for environment sync.

package activity

import (
	"encoding/json/v2"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestSyncResourcesToEnvironmentOutcomes(t *testing.T) {
	paths := []string{"/api/container-registries/sync"}
	cases := []struct {
		name     string
		failures map[string]string
		groups   string
	}{
		{name: "success"},
		{name: "registry failure", failures: map[string]string{paths[0]: "status"}, groups: "container registries"},
		{name: "malformed response", failures: map[string]string{paths[0]: "malformed"}, groups: "container registries"},
		{name: "agent reports failure", failures: map[string]string{paths[0]: "false"}, groups: "container registries"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			require.NoError(t, db.AutoMigrate(&environment.Environment{}, &registry.ContainerRegistry{}))
			sqlDB, err := db.DB.DB()
			require.NoError(t, err)
			sqlDB.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, sqlDB.Close()) })
			crypto.InitEncryption(&crypto.Config{EncryptionKey: "test-encryption-key-for-testing-32bytes-min", Environment: "test"})
			var mu sync.Mutex
			var calls []string
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				calls = append(calls, r.URL.Path)
				mu.Unlock()
				if r.URL.Path == "/api/app-version" {
					require.NoError(t, json.MarshalWrite(w, version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint}))
					return
				}
				switch tc.failures[r.URL.Path] {
				case "status":
					http.Error(w, "private-agent-response", http.StatusUnauthorized)
				case "malformed":
					_, _ = w.Write([]byte("private-agent-response"))
				case "false":
					_, _ = w.Write([]byte(`{"success":false,"data":{"message":"private-agent-response"}}`))
				default:
					_, _ = w.Write([]byte(`{"success":true}`))
				}
			}))
			defer server.Close()
			require.NoError(t, db.Create(&environment.Environment{ID: "remote", Name: "Remote", ApiUrl: server.URL, AccessToken: new("agent-token"), Enabled: true}).Error)
			service := environment.NewEnvironmentService(db, server.Client(), nil, nil, nil, nil)
			activityService := NewActivityService(db, nil)
			id, err := service.SyncResourcesToEnvironment(t.Context(), "remote", &user.Actor{ID: "operator", Username: "operator"}, activityService)
			require.NotEmpty(t, id)
			var recorded Activity
			require.NoError(t, db.First(&recorded, "id = ?", id).Error)
			require.NotNil(t, recorded.EndedAt)
			require.NotNil(t, recorded.DurationMs)
			require.Equal(t, "remote", recorded.EnvironmentID)
			if tc.groups == "" {
				require.NoError(t, err)
				require.Equal(t, activity.StatusSuccess, recorded.Status)
				require.Equal(t, "Environment synced successfully", recorded.LatestMessage)
				require.Nil(t, recorded.Error)
			} else {
				require.EqualError(t, err, "Failed to sync "+tc.groups+". Other resource groups may have synced successfully. Check the manager logs, correct the failed sync, and retry.")
				require.Equal(t, activity.StatusFailed, recorded.Status)
				require.NotNil(t, recorded.Error)
				require.Equal(t, err.Error(), *recorded.Error)
				require.Equal(t, err.Error(), recorded.LatestMessage)
				require.NotContains(t, err.Error(), "private-agent-response")
				require.NotContains(t, err.Error(), "agent-token")
			}
			mu.Lock()
			defer mu.Unlock()
			require.Equal(t, append([]string{"/api/app-version"}, paths...), calls)
		})
	}
}
