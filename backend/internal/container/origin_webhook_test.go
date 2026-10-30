// Source: docklet-next/issues/253
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original retained webhook producer proof for the container origin effect guard.

package container_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	containers "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/webhook"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestWebhookComposeRedeployRefusedBeforeDockerEffects(t *testing.T) {
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "webhook.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.NewString()})
	var effects atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Api-Version", "1.53")
		if r.Method == http.MethodHead || strings.HasSuffix(r.URL.Path, "/_ping") {
			return
		}
		if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/json") {
			require.NoError(t, json.NewEncoder(w).Encode(map[string]any{"Id": strings.Repeat("a", 64), "Name": "/app", "Config": map[string]any{"Image": "registry.example.com/app:1.0.0", "Labels": map[string]string{"com.docker.compose.project": "example", "com.docker.compose.service": "app"}}, "State": map[string]any{"Running": true}}))
			return
		}
		effects.Add(1)
		w.WriteHeader(500)
	}))
	t.Cleanup(server.Close)
	client, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.53"), client.WithHTTPClient(server.Client()))
	require.NoError(t, err)
	t.Cleanup(func() { _ = client.Close() })
	events := event.NewEventService(db, &config.Config{}, nil)
	service := containers.NewContainerService(events, docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(client), nil, nil, nil)
	producer := webhook.NewWebhookService(db, service, nil, nil, events, nil)
	_, token, err := producer.CreateWebhook(t.Context(), "Example", "container", "redeploy", strings.Repeat("a", 64), "0", user.SystemUser)
	require.NoError(t, err)
	require.NoError(t, producer.TriggerByToken(t.Context(), token))
	require.NoError(t, producer.DrainActions(t.Context()))
	require.Zero(t, effects.Load(), "a Compose child cannot reach pull, create, stop or restart through a webhook")
	var rows []event.Event
	require.NoError(t, db.Find(&rows).Error)
	body, err := json.Marshal(rows)
	require.NoError(t, err)
	require.Contains(t, string(body), "container.origin-locked")
}
