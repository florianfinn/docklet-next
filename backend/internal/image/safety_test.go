// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package image

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"go.getarcane.app/sys/crypto"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"uuid"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	registryService "github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func enableImageSafetyForTest(t *testing.T, s *ImageService) {
	t.Helper()
	if s.db != nil {
		require.NoError(t, s.db.AutoMigrate(&guard.Protection{}))
	}
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	s.SetImageAdmission(gate, nil)
}
func safetyFixture(t *testing.T, daemon http.HandlerFunc) (*ImageService, *database.DB, *activity.ActivityService) {
	t.Helper()
	db, err := database.Initialize(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "image.db")), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, db.Close()) })
	server := httptest.NewServer(daemon)
	t.Cleanup(server.Close)
	d := &docker.DockerClientService{Client: newTestDockerClientInternal(t, server)}
	a := activity.NewActivityService(db, nil)
	s := NewImageService(db, d, nil, nil, nil, event.NewEventService(db, nil, nil), nil, a)
	enableImageSafetyForTest(t, s)
	return s, db, a
}
func TestPR1MutationRoutesRefuseBeforeDaemon(t *testing.T) {
	var writes atomic.Int32
	s, _, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		writes.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})
	err := s.TagImage(t.Context(), "source", imageDTO.TagRequest{Repository: "DOCKLET-RETENTION.INVALID/points/p/s", Tag: "saved"}, user.SystemUser)
	assert.Error(t, err)
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	body := `[{"Config":"config.json","RepoTags":["docklet-retention.invalid/points/p/s:saved"],"Layers":[]}]`
	require.NoError(t, tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0600, Size: int64(len(body))}))
	_, err = io.WriteString(tw, body)
	require.NoError(t, err)
	require.NoError(t, tw.Close())
	_, err = s.LoadImageFromReader(t.Context(), bytes.NewReader(archive.Bytes()), "archive.tar", user.SystemUser, 1<<20)
	assert.Error(t, err)
	require.Zero(t, writes.Load(), "namespace guards must run before ImageTag/ImageLoad")
}
func TestPR1PullSecretAbsentFromStreamAndActivity(t *testing.T) {
	sentinel := uuid.New().String()
	var pulls atomic.Int32
	s, db, activities := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if imageDockerTestPathInternal(r.URL.Path) == "/images/create" {
			pulls.Add(1)
			_, _ = fmt.Fprintf(w, `{"status":%q,"errorDetail":{"message":%q},"error":%q}`, sentinel, sentinel, sentinel)
			return
		}
		http.Error(w, "unexpected", 500)
	})
	settingsService, err := settings.NewSettingsService(t.Context(), db)
	require.NoError(t, err)
	router := echo.New()
	actor := user.Actor{ID: "operator", Username: "operator"}
	router.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			ctx := context.WithValue(c.Request().Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
			ctx = context.WithValue(ctx, user.CurrentUserContextKey{}, &actor)
			c.SetRequest(c.Request().WithContext(ctx))
			return next(c)
		}
	})
	api := humaecho.NewWithGroup(router, router.Group("/api"), huma.DefaultConfig("images", "test"))
	RegisterImages(api, s.dockerService, s, nil, settingsService, activities, nil, handlerutil.NewActivityAppContext(t.Context()))
	req := httptest.NewRequest("POST", "/api/environments/0/images/pull", strings.NewReader(`{"imageName":"registry.example.com/app:1.0.0"}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, req)
	require.Equal(t, 200, rec.Code)
	require.EqualValues(t, 1, pulls.Load())
	require.Contains(t, rec.Body.String(), `"code":"image.pull-failed"`)
	require.NotContains(t, rec.Body.String(), sentinel)
	require.NotContains(t, rec.Body.String(), `"type":"done"`)
	for _, table := range []string{"activities", "activity_messages", "events"} {
		var rows []map[string]any
		require.NoError(t, db.Table(table).Find(&rows).Error)
		encoded, e := json.Marshal(rows)
		require.NoError(t, e)
		require.NotContains(t, string(encoded), sentinel)
	}
}
func TestPR1PullInlineCredentialsHaveZeroDaemonCalls(t *testing.T) {
	var calls atomic.Int32
	s, _, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) { calls.Add(1); _, _ = io.WriteString(w, `{}`) })
	var opts imageDTO.PullOptions
	require.NoError(t, json.Unmarshal([]byte(`{"imageName":"registry.example.com/app:1.0.0","auth":null}`), &opts))
	assert.Error(t, opts.Validate())
	assert.Error(t, s.PullImage(t.Context(), "registry.example.com/app:1.0.0", io.Discard, user.SystemUser, []containerregistry.Credential{{URL: "registry.example.com", Username: "credential-identity", Token: uuid.New().String(), Enabled: true}}))
	require.Zero(t, calls.Load())
}
func TestPR1PullProgressWhitelist(t *testing.T) {
	sentinel := uuid.New().String()
	s, _, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"status":%q}`, sentinel)
	})
	var progress bytes.Buffer
	require.NoError(t, s.PullImage(t.Context(), "registry.example.com/app:1.0.0", &progress, user.SystemUser, nil))
	require.NotContains(t, progress.String(), sentinel)
	require.Contains(t, progress.String(), "Pulling image")
}

func TestPR1UnavailableStoredCredentialHasZeroDaemonCalls(t *testing.T) {
	var calls atomic.Int32
	s, db, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{}`)
	})
	s.registryService = registryService.NewContainerRegistryService(db, nil, nil, nil)
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: uuid.New().String()})
	ciphertext, err := crypto.Encrypt(uuid.New().String())
	require.NoError(t, err)
	reg := &registryService.ContainerRegistry{URL: "https://registry.example.com", Username: "credential-identity", Token: ciphertext, RegistryType: "generic", Enabled: true}
	require.NoError(t, db.Create(reg).Error)
	require.NoError(t, db.Model(reg).Update("token", uuid.New().String()).Error)
	assert.Error(t, s.PullImage(t.Context(), "registry.example.com/app:1.0.0", io.Discard, user.SystemUser, nil))
	require.Zero(t, calls.Load(), "unusable stored authentication must not become an anonymous pull")
}

func TestPR1ComposePullRemainsOutsideImageLease(t *testing.T) {
	var calls atomic.Int32
	s, _, _ := safetyFixture(t, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		require.Empty(t, r.Header.Get("X-Registry-Auth"))
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"status":"downloaded"}`)
	})
	release, err := s.safety.Acquire(t.Context())
	require.NoError(t, err)
	defer release()
	assert.Error(t, s.PullImage(t.Context(), "registry.example.com/app:1.0.0", io.Discard, user.SystemUser, nil))
	require.Zero(t, calls.Load())
	assert.NoError(t, s.PullComposeImage(t.Context(), "registry.example.com/app:1.0.0", io.Discard, user.SystemUser, []containerregistry.Credential{{URL: "registry.example.com", Username: "ignored", Token: uuid.New().String()}}))
	require.EqualValues(t, 1, calls.Load(), "ordinary Compose pull must execute while M10 admission is busy")
}
