// Source: docklet-next/issues/493
// Revision: 6a116e7af8c27389e48fa5c31c52c19670bad974
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original guard that tools image pulls keep stored registry credentials.

package volume

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
)

// A missing tools image is pulled with Docklet's stored credentials for its
// registry, as before the shared resolver (private overrides keep working).
func TestHelperImagePullUsesStoredRegistryCredentials(t *testing.T) {
	gdb, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, gdb.AutoMigrate(&registry.ContainerRegistry{}, &kv.KVEntry{}))
	crypto.InitEncryption(&crypto.Config{Environment: string(config.AppEnvironmentTest), EncryptionKey: "test-encryption-key-for-testing-32bytes-min"})
	db := &database.DB{DB: gdb}
	token, err := crypto.Encrypt("synthetic-token")
	require.NoError(t, err)
	require.NoError(t, gdb.Create(&registry.ContainerRegistry{URL: "ghcr.io", Username: "synthetic-user", Token: token, Enabled: true, RegistryType: registry.RegistryTypeGeneric}).Error)

	var pulled atomic.Bool
	var auth atomic.Value
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/json") && strings.Contains(r.URL.Path, "/images/"):
			if !pulled.Load() {
				http.Error(w, `{"message":"No such image"}`, http.StatusNotFound)
				return
			}
			_, _ = w.Write([]byte(`{"Id":"sha256:local-image"}`))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/images/create"):
			auth.Store(r.Header.Get("X-Registry-Auth"))
			if r.Header.Get("X-Registry-Auth") == "" {
				_, _ = w.Write([]byte(`{"error":"pull access denied"}` + "\n"))
				return
			}
			pulled.Store(true)
			_, _ = w.Write([]byte(`{"status":"pulled"}` + "\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(server.Close)
	cli := newVolumeServiceTestDockerClientInternal(t, server)
	svc := &VolumeService{
		dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli),
		imageService:  image.NewImageService(db, nil, registry.NewContainerRegistryService(db, nil, kv.NewKVService(db), nil), nil, nil, nil, nil, nil),
	}
	ref, err := svc.getVolumeHelperImageInternal(t.Context(), cli)
	require.NoError(t, err)
	require.NotEmpty(t, ref)
	require.NotEmpty(t, auth.Load())
}
