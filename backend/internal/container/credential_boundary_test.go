// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Actual create and recreate pull boundaries reject unusable saved authentication.

package container

import (
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	images "github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestCreateAndRecreateSavedAuthenticationNeverFallsBackToAnonymous(t *testing.T) {
	db := setupProjectTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&registry.ContainerRegistry{}))
	var pulls, creates atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/images/") && strings.HasSuffix(r.URL.Path, "/json") {
			w.WriteHeader(404)
			_, _ = w.Write([]byte(`{"message":"not found"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/images/create") {
			pulls.Add(1)
			_, _ = w.Write([]byte(`{"status":"downloaded"}`))
			return
		}
		if strings.HasSuffix(r.URL.Path, "/containers/create") {
			creates.Add(1)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()
	daemon := &docker.DockerClientService{Client: newTestDockerClientInternal(t, server)}
	cfg, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	defer cfg.Stop(t.Context())
	events := event.NewEventService(db, nil, nil)
	regs := registry.NewContainerRegistryService(db, nil, nil, nil)
	imgs := images.NewImageService(db, daemon, regs, nil, nil, events, cfg, nil)
	svc := NewContainerService(events, daemon, imgs, cfg, nil)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.NewString()})
	row := &registry.ContainerRegistry{URL: "https://registry.example.com", Username: "synthetic-user", Token: uuid.NewString(), Enabled: true, RegistryType: "generic"}
	require.NoError(t, db.Create(row).Error)
	_, err = svc.CreateContainer(t.Context(), &container.Config{Image: "registry.example.com/app:1.0.0"}, &container.HostConfig{}, nil, "example-app", user.Actor{ID: "operator", Username: "operator"}, nil)
	require.Zero(t, pulls.Load())
	require.Zero(t, creates.Load())
	require.Error(t, err)
	err = svc.pullRedeployImageInternal(t.Context(), daemon.Client, "registry.example.com/app:1.0.0", "example-container", "example-app", "redeploy", nil, user.Actor{ID: "operator", Username: "operator"})
	require.Zero(t, pulls.Load())
	require.Zero(t, creates.Load())
	require.Error(t, err)
}
