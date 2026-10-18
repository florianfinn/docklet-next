// Source: getarcaneapp/arcane/backend/internal/environment/service_test.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations; SQLite temp-file persistence fixture.

package environment

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"github.com/coder/websocket"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/snippets"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/internal/user"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/entityjobs"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/florianfinn/docklet-next/types/environment"
	"github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestEnvironmentService_OverlappingHealthCheckIsSkippedInternal(t *testing.T) {
	gate := newAdmissionGateForEnvironmentTestInternal(t)

	key := scheduler.AdmissionKey{Scope: environmentHealthAdmissionScopeInternal, ID: "environment-id"}
	lease, admitted, err := gate.TryAcquire(t.Context(), key)
	require.NoError(t, err)
	require.True(t, admitted)

	service := &EnvironmentService{jobs: entityjobs.New(environmentHealthJobPrefix, environmentHealthAdmissionScopeInternal)}
	require.NoError(t, service.SetScheduler(t.Context(), &environmentTestSchedulerInternal{}, gate))
	outcome, runErr := service.runHealthCheckInternal(t.Context(), "environment-id")
	require.NoError(t, runErr)
	require.Equal(t, scheduler.Skipped, outcome.Status)
	lease.Release(t.Context())
}

type environmentTestSchedulerInternal struct {
	submitted []scheduler.Request
	added     []string
	removed   []string
}

func (s *environmentTestSchedulerInternal) AddJob(_ context.Context, job scheduler.Job) error {
	s.added = append(s.added, job.Name())
	return nil
}

func (s *environmentTestSchedulerInternal) RemoveJob(_ context.Context, name string) {
	s.removed = append(s.removed, name)
}

func (s *environmentTestSchedulerInternal) HasJob(_ string) bool {
	return false
}

func newAdmissionGateForEnvironmentTestInternal(t testing.TB) *runs.Admission {
	t.Helper()
	runtime := francistest.New(t)
	gate := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, gate.Register(runtime))
	francistest.Start(t, runtime)
	return gate
}

// Project imports environment, so these rows avoid an import cycle in tests.
type testProjectRow struct {
	database.BaseModel
	Name   string
	Path   string
	Status string
}

func (testProjectRow) TableName() string { return "projects" }

func setupEnvironmentServiceTestDB(t *testing.T) *database.DB {
	t.Helper()

	dsn := fmt.Sprintf("file:%s?mode=memory&cache=shared", strings.NewReplacer("/", "_", " ", "_").Replace(t.Name()))
	db, err := gorm.Open(testsqlite.Dialector(t, dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(
		&Environment{},
		&registry.ContainerRegistry{},
		&settings.SettingVariable{},
		&user.User{},
		&apikey.ApiKey{},
		&testProjectRow{},
	))

	sqlDB, err := db.DB()
	require.NoError(t, err)
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	testCfg := &config.Config{
		EncryptionKey: "test-encryption-key-for-testing-32bytes-min",
		Environment:   "test",
	}
	crypto.InitEncryption(&crypto.Config{
		EncryptionKey: testCfg.EncryptionKey,
		Environment:   string(testCfg.Environment),
		AgentMode:     testCfg.AgentMode,
	})

	return &database.DB{DB: db}
}

func createTestEnvironmentServiceUser(t *testing.T, ctx context.Context, userService *user.UserService, id string) *user.User {
	t.Helper()

	localUser := &user.User{
		ID:       id,
		Username: "user-" + id,
	}

	created, err := userService.CreateUser(ctx, localUser)
	require.NoError(t, err)
	return created
}

func createTestEnvironment(t *testing.T, db *database.DB, id, apiURL string, accessToken *string) {
	t.Helper()
	createNamedTestEnvironmentInternal(t, db, id, "env-"+id, apiURL, accessToken)
}

func createNamedTestEnvironmentInternal(t *testing.T, db *database.DB, id, name, apiURL string, accessToken *string) {
	t.Helper()

	now := time.Now()
	env := &Environment{
		ID:          id,
		CreatedAt:   now,
		UpdatedAt:   &now,
		Name:        name,
		ApiUrl:      apiURL,
		Status:      string(EnvironmentStatusOnline),
		Enabled:     true,
		AccessToken: accessToken,
	}

	require.NoError(t, db.WithContext(t.Context()).Create(env).Error)
}

func TestEnvironmentService_RegenerateEnvironmentApiKeyKeepsPairing(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	keys := apikey.NewApiKeyService(db, user.NewUserService(db, nil, nil), nil)
	old, err := keys.CreateEnvironmentApiKey(ctx, "env-rotation")
	require.NoError(t, err)
	createTestEnvironment(t, db, "env-rotation", "http://agent.example", &old.Key)
	require.NoError(t, db.WithContext(ctx).Model(&Environment{}).Where("id = ?", "env-rotation").Updates(map[string]any{
		"api_key_id": old.ID,
		"is_edge":    true,
	}).Error)

	service := NewEnvironmentService(db, nil, nil, nil, nil, keys)
	env, err := service.GetEnvironmentByID(ctx, "env-rotation")
	require.NoError(t, err)
	rotated, err := service.RegenerateEnvironmentApiKey(ctx, env, "user-1", "operator")
	require.NoError(t, err)
	require.NotEqual(t, old.Key, rotated)

	var persisted Environment
	require.NoError(t, db.WithContext(ctx).First(&persisted, "id = ?", "env-rotation").Error)
	require.Equal(t, rotated, *persisted.AccessToken)
	require.NotEqual(t, old.ID, *persisted.ApiKeyID)
	resolved, err := service.ResolveEdgeEnvironmentByToken(ctx, rotated)
	require.NoError(t, err)
	require.Equal(t, persisted.ID, resolved)
	_, err = keys.GetApiKey(ctx, old.ID)
	require.ErrorIs(t, err, apikey.ErrApiKeyNotFound)
}

func createTestEnvironmentWithState(t *testing.T, db *database.DB, id, apiURL, status string, isEdge bool, accessToken *string) {
	t.Helper()

	now := time.Now()
	env := &Environment{
		ID:          id,
		CreatedAt:   now,
		UpdatedAt:   &now,
		Name:        "env-" + id,
		ApiUrl:      apiURL,
		Status:      status,
		Enabled:     true,
		IsEdge:      isEdge,
		AccessToken: accessToken,
	}

	require.NoError(t, db.WithContext(t.Context()).Create(env).Error)
}

func createTestRegistry(t *testing.T, db *database.DB, id string) {
	t.Helper()

	encryptedToken, err := crypto.Encrypt("registry-token")
	require.NoError(t, err)

	now := time.Now()
	localRegistry := &registry.ContainerRegistry{
		BaseModel: database.BaseModel{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: &now,
		},
		URL:          "registry.example.com",
		Username:     "registry-user",
		Token:        encryptedToken,
		Enabled:      true,
		Insecure:     false,
		RegistryType: registry.RegistryTypeGeneric,
		CreatedAt:    now,
		UpdatedAt:    now,
	}

	require.NoError(t, db.WithContext(t.Context()).Create(localRegistry).Error)
}

func createTestECRRegistry(t *testing.T, db *database.DB, id string) {
	t.Helper()

	encryptedSecret, err := crypto.Encrypt("aws-secret")
	require.NoError(t, err)

	now := time.Now()
	localRegistry := &registry.ContainerRegistry{
		BaseModel: database.BaseModel{
			ID:        id,
			CreatedAt: now,
			UpdatedAt: &now,
		},
		URL:                "123456789012.dkr.ecr.us-east-1.amazonaws.com",
		Enabled:            true,
		RegistryType:       registry.RegistryTypeECR,
		AWSAccessKeyID:     "AKIA1234567890EXAMPLE",
		AWSSecretAccessKey: encryptedSecret,
		AWSRegion:          "us-east-1",
		CreatedAt:          now,
		UpdatedAt:          now,
	}

	require.NoError(t, db.WithContext(t.Context()).Create(localRegistry).Error)
}

func encryptSecretForTest(t *testing.T, value string) string {
	t.Helper()
	encrypted, err := crypto.Encrypt(value)
	require.NoError(t, err)
	return encrypted
}

func TestEnvironmentService_SyncRegistriesToRemoteEnvironments_SyncsEligibleRemotes(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestRegistry(t, db, "reg-1")

	var env1Calls atomic.Int32
	env1Token := "token-1"
	env1Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		if !assert.Equal(t, http.MethodPost, r.Method) {
			return
		}
		if !assert.Equal(t, "/api/container-registries/sync", r.URL.Path) {
			return
		}
		if !assert.Equal(t, env1Token, r.Header.Get("X-API-Key")) {
			return
		}
		if !assert.Equal(t, env1Token, r.Header.Get("X-Arcane-Agent-Token")) {
			return
		}

		var syncReq containerregistry.SyncRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&syncReq)) {
			return
		}
		if !assert.Len(t, syncReq.Registries, 1) {
			return
		}
		env1Calls.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	}))
	defer env1Server.Close()

	var env2Calls atomic.Int32
	env2Token := "token-2"
	env2Server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		if !assert.Equal(t, http.MethodPost, r.Method) {
			return
		}
		if !assert.Equal(t, "/api/container-registries/sync", r.URL.Path) {
			return
		}
		if !assert.Equal(t, env2Token, r.Header.Get("X-API-Key")) {
			return
		}
		if !assert.Equal(t, env2Token, r.Header.Get("X-Arcane-Agent-Token")) {
			return
		}

		var syncReq containerregistry.SyncRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&syncReq)) {
			return
		}
		if !assert.Len(t, syncReq.Registries, 1) {
			return
		}
		env2Calls.Add(1)

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	}))
	defer env2Server.Close()

	createTestEnvironment(t, db, "0", "http://localhost:3552", nil) // local env should be excluded
	createTestEnvironment(t, db, "env-1", env1Server.URL, &env1Token)
	createTestEnvironment(t, db, "env-2", env2Server.URL, &env2Token)

	err := svc.SyncRegistriesToRemoteEnvironments(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, env1Calls.Load())
	require.EqualValues(t, 1, env2Calls.Load())
}

func TestEnvironmentService_SyncRegistriesToEnvironment_IncludesECRFields(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestECRRegistry(t, db, "reg-ecr")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		if !assert.Equal(t, http.MethodPost, r.Method) {
			return
		}
		if !assert.Equal(t, "/api/container-registries/sync", r.URL.Path) {
			return
		}

		var syncReq containerregistry.SyncRequest
		if !assert.NoError(t, json.NewDecoder(r.Body).Decode(&syncReq)) {
			return
		}
		if !assert.Len(t, syncReq.Registries, 1) {
			return
		}

		syncedRegistry := syncReq.Registries[0]
		if !assert.Equal(t, registry.RegistryTypeECR, syncedRegistry.RegistryType) {
			return
		}
		if !assert.Equal(t, "123456789012.dkr.ecr.us-east-1.amazonaws.com", syncedRegistry.URL) {
			return
		}
		if !assert.Equal(t, "AKIA1234567890EXAMPLE", syncedRegistry.AWSAccessKeyID) {
			return
		}
		if !assert.Equal(t, "aws-secret", syncedRegistry.AWSSecretAccessKey) {
			return
		}
		if !assert.Equal(t, "us-east-1", syncedRegistry.AWSRegion) {
			return
		}
		if !assert.Empty(t, syncedRegistry.Username) {
			return
		}
		if !assert.Empty(t, syncedRegistry.Token) {
			return
		}

		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	}))
	defer server.Close()

	createTestEnvironment(t, db, "env-1", server.URL, new("token-1"))

	err := svc.SyncRegistriesToEnvironment(ctx, "env-1")
	require.NoError(t, err)
}

func TestEnvironmentService_SyncRegistriesToRemoteEnvironments_SkipsRemoteWithoutAccessToken(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestRegistry(t, db, "reg-1")

	var syncCalls atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		syncCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	}))
	defer server.Close()

	createTestEnvironment(t, db, "env-auth", server.URL, new("token-with-auth"))
	createTestEnvironment(t, db, "env-no-token", "http://127.0.0.1:1", nil)

	err := svc.SyncRegistriesToRemoteEnvironments(ctx)
	require.NoError(t, err)
	require.EqualValues(t, 1, syncCalls.Load())
}

func TestEnvironmentService_SyncRegistriesToRemoteEnvironments_ReportsFailuresButContinues(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestRegistry(t, db, "reg-1")

	var successCalls atomic.Int32
	successServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		successCalls.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"success":true,"data":{"message":"ok"}}`))
	}))
	defer successServer.Close()

	createTestEnvironment(t, db, "env-success", successServer.URL, new("token-success"))
	createTestEnvironment(t, db, "env-fail", "http://127.0.0.1:1", new("token-fail"))

	err := svc.SyncRegistriesToRemoteEnvironments(ctx)
	require.Error(t, err)
	require.Contains(t, err.Error(), "failed to sync registries to 1 remote environment")
	require.EqualValues(t, 1, successCalls.Load())
}

func TestEnvironmentService_ReconcileEdgeStatusesOnStartup(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironmentWithState(t, db, "edge-online", "edge://online", string(EnvironmentStatusOnline), true, nil)
	createTestEnvironmentWithState(t, db, "edge-error", "edge://error", string(EnvironmentStatusError), true, nil)
	createTestEnvironmentWithState(t, db, "edge-pending", "edge://pending", string(EnvironmentStatusPending), true, nil)
	createTestEnvironmentWithState(t, db, "remote-http", "http://remote.example", string(EnvironmentStatusOnline), false, nil)

	err := svc.ReconcileEdgeStatusesOnStartup(ctx)
	require.NoError(t, err)

	var edgeOnline Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-online").First(&edgeOnline).Error)
	require.Equal(t, string(EnvironmentStatusOffline), edgeOnline.Status)

	var edgeError Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-error").First(&edgeError).Error)
	require.Equal(t, string(EnvironmentStatusOffline), edgeError.Status)

	var edgePending Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-pending").First(&edgePending).Error)
	require.Equal(t, string(EnvironmentStatusPending), edgePending.Status)

	var remoteHTTP Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "remote-http").First(&remoteHTTP).Error)
	require.Equal(t, string(EnvironmentStatusOnline), remoteHTTP.Status)
}

func TestEnvironmentService_UpdateEnvironmentConnectionState(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironmentWithState(t, db, "edge-runtime", "edge://runtime", string(EnvironmentStatusOffline), true, nil)

	err := svc.UpdateEnvironmentConnectionState(ctx, "edge-runtime", true)
	require.NoError(t, err)

	var env Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-runtime").First(&env).Error)
	require.Equal(t, string(EnvironmentStatusPending), env.Status)
	require.NotNil(t, env.LastSeen)

	lastSeen := env.LastSeen

	err = svc.UpdateEnvironmentConnectionState(ctx, "edge-runtime", false)
	require.NoError(t, err)

	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-runtime").First(&env).Error)
	require.Equal(t, string(EnvironmentStatusPending), env.Status)
	require.NotNil(t, env.LastSeen)
	require.Equal(t, *lastSeen, *env.LastSeen)
}

func TestEnvironmentService_UpdateEnvironmentStatusInternal_PromotesPendingDirectEnv(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironmentWithState(t, db, "direct-pending", "http://agent:3553", string(EnvironmentStatusPending), false, nil)

	require.NoError(t, svc.updateEnvironmentStatusInternal(ctx, "direct-pending", string(EnvironmentStatusOnline)))

	var env Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "direct-pending").First(&env).Error)
	require.Equal(t, string(EnvironmentStatusOnline), env.Status)
	require.NotNil(t, env.LastSeen)
}

func TestEnvironmentService_UpdateEnvironmentStatusInternal_DoesNotDemotePendingDirectEnvOnFailedTick(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironmentWithState(t, db, "direct-pending", "http://agent:3553", string(EnvironmentStatusPending), false, nil)

	// A failed health check leaves an unpaired Direct environment pending.
	require.NoError(t, svc.updateEnvironmentStatusInternal(ctx, "direct-pending", string(EnvironmentStatusOffline)))
	require.NoError(t, svc.updateEnvironmentStatusInternal(ctx, "direct-pending", string(EnvironmentStatusError)))

	var env Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "direct-pending").First(&env).Error)
	require.Equal(t, string(EnvironmentStatusPending), env.Status)
	require.Nil(t, env.LastSeen)
}

func TestEnvironmentService_UpdateEnvironmentStatusInternal_LeavesPendingEdgeEnvAlone(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironmentWithState(t, db, "edge-pending", "edge://agent", string(EnvironmentStatusPending), true, nil)

	// Edge envs in pending must complete pairing via the agent's outbound tunnel;
	// a manager-side reachability tick must NOT promote them.
	require.NoError(t, svc.updateEnvironmentStatusInternal(ctx, "edge-pending", string(EnvironmentStatusOnline)))

	var env Environment
	require.NoError(t, db.WithContext(ctx).Where("id = ?", "edge-pending").First(&env).Error)
	require.Equal(t, string(EnvironmentStatusPending), env.Status)
	require.Nil(t, env.LastSeen)
}

func TestEnvironmentService_ResolveEdgeEnvironmentByToken_CachesAndInvalidatesOnUpdate(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	oldToken := "edge-token-old"
	newToken := "edge-token-new"
	createTestEnvironmentWithState(t, db, "edge-auth", "edge://auth", string(EnvironmentStatusPending), true, &oldToken)

	envID, err := svc.ResolveEdgeEnvironmentByToken(ctx, oldToken)
	require.NoError(t, err)
	require.Equal(t, "edge-auth", envID)

	_, err = svc.UpdateEnvironment(ctx, "edge-auth", map[string]any{"access_token": newToken}, nil, nil)
	require.NoError(t, err)

	_, err = svc.ResolveEdgeEnvironmentByToken(ctx, oldToken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid agent token")

	envID, err = svc.ResolveEdgeEnvironmentByToken(ctx, newToken)
	require.NoError(t, err)
	require.Equal(t, "edge-auth", envID)

	require.NoError(t, svc.DeleteEnvironment(ctx, "edge-auth", nil, nil))
	_, err = svc.ResolveEdgeEnvironmentByToken(ctx, newToken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid agent token")
}

func TestEnvironmentService_UpdateEnvironment_ClearingAccessTokenInvalidatesCache(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	oldToken := "edge-token-clear"
	createTestEnvironmentWithState(t, db, "edge-auth-clear", "edge://auth-clear", string(EnvironmentStatusPending), true, &oldToken)

	envID, err := svc.ResolveEdgeEnvironmentByToken(ctx, oldToken)
	require.NoError(t, err)
	require.Equal(t, "edge-auth-clear", envID)

	_, err = svc.UpdateEnvironment(ctx, "edge-auth-clear", map[string]any{"access_token": nil}, nil, nil)
	require.NoError(t, err)

	cachedEnvID, ok := svc.edgeTokens.environmentID(oldToken).Get()
	require.False(t, ok)
	require.Empty(t, cachedEnvID)

	_, tokenStillCached := svc.edgeTokens.byToken.Peek(oldToken)
	svc.edgeTokens.mu.RLock()
	_, reverseIndexStillCached := svc.edgeTokens.byEnvID["edge-auth-clear"]
	svc.edgeTokens.mu.RUnlock()

	require.False(t, tokenStillCached)
	require.False(t, reverseIndexStillCached)

	_, err = svc.ResolveEdgeEnvironmentByToken(ctx, oldToken)
	require.Error(t, err)
	require.Contains(t, err.Error(), "invalid agent token")
}

func TestEnvironmentServiceUpdateEnvironmentRejectsTargetChangeWithStoredTokenInternal(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	oldToken := "stored-agent-token"
	createTestEnvironment(t, db, "env-target", "http://agent.example:3553", &oldToken)
	updates := map[string]any{"api_url": "http://attacker.example:3553"}

	_, err := svc.UpdateEnvironment(ctx, "env-target", updates, nil, nil)
	require.ErrorIs(t, err, common.ErrValidation)
	require.NotContains(t, updates, "updated_at")

	var stored Environment
	require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", "env-target").Error)
	require.Equal(t, "http://agent.example:3553", stored.ApiUrl)
	require.Equal(t, oldToken, *stored.AccessToken)
}

func TestEnvironmentServiceUpdateEnvironmentAllowsTargetChangeWithExplicitTokenInternal(t *testing.T) {
	for _, tc := range []struct {
		name  string
		token string
	}{
		{name: "reenter existing token", token: "stored-agent-token"},
		{name: "use replacement token", token: "replacement-agent-token"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			ctx := t.Context()
			db := setupEnvironmentServiceTestDB(t)
			svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

			oldToken := "stored-agent-token"
			createTestEnvironment(t, db, "env-target", "http://agent.example:3553", &oldToken)

			updated, err := svc.UpdateEnvironment(ctx, "env-target", map[string]any{
				"api_url":      "http://replacement.example:3553",
				"access_token": tc.token,
			}, nil, nil)
			require.NoError(t, err)
			require.Equal(t, "http://replacement.example:3553", updated.ApiUrl)
			require.Equal(t, tc.token, *updated.AccessToken)

			var stored Environment
			require.NoError(t, db.WithContext(ctx).First(&stored, "id = ?", "env-target").Error)
			require.Equal(t, updated.ApiUrl, stored.ApiUrl)
			require.Equal(t, tc.token, *stored.AccessToken)
		})
	}
}

func TestEnvironmentService_getCachedEnvironmentIDForTokenInternal_ExpiresAndCleansReverseIndex(t *testing.T) {
	svc := NewEnvironmentService(nil, nil, nil, nil, nil, nil)
	svc.edgeTokens.mu.Lock()
	svc.edgeTokens.byEnvID["env-expired"] = "expired-token"
	svc.edgeTokens.byToken.SetWithTTL("expired-token", "env-expired", time.Nanosecond)
	time.Sleep(time.Millisecond)
	svc.edgeTokens.mu.Unlock()

	envID, ok := svc.edgeTokens.environmentID("expired-token").Get()
	require.False(t, ok)
	require.Empty(t, envID)

	_, tokenStillCached := svc.edgeTokens.byToken.Peek("expired-token")
	svc.edgeTokens.mu.RLock()
	_, reverseIndexStillCached := svc.edgeTokens.byEnvID["env-expired"]
	svc.edgeTokens.mu.RUnlock()

	require.False(t, tokenStillCached)
	require.False(t, reverseIndexStillCached)
}

func TestEnvironmentService_ResolveEnvironmentByAccessToken(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	accessToken := "remote-token"
	createNamedTestEnvironmentInternal(t, db, "env-remote", "Remote Alpha", "http://remote.example", &accessToken)

	env, err := svc.ResolveEnvironmentByAccessToken(ctx, accessToken)
	require.NoError(t, err)
	require.NotNil(t, env)
	require.Equal(t, "env-remote", env.ID)
	require.Equal(t, "Remote Alpha", env.Name)

	_, err = svc.ResolveEnvironmentByAccessToken(ctx, "missing-token")
	require.Error(t, err)
	require.ErrorIs(t, err, ErrInvalidEnvironmentAccessToken)
}

func TestEnvironmentService_GenerateDeploymentSnippets_ExplicitlyUsePollTransport(t *testing.T) {
	svc := NewEnvironmentService(nil, nil, nil, nil, nil, nil)

	standard, err := svc.GenerateDeploymentSnippets(t.Context(), "env-1", "https://manager.example.com", "https://agent.example.com", "token-123")
	require.NoError(t, err)
	require.NotNil(t, standard)
	require.NotContains(t, standard.DockerRun, "EDGE_TRANSPORT=websocket")
	require.NotContains(t, standard.DockerCompose, "EDGE_TRANSPORT=websocket")
	require.Contains(t, standard.DockerRun, "EDGE_TRANSPORT=poll")
	require.Contains(t, standard.DockerCompose, "EDGE_TRANSPORT=poll")
	require.Contains(t, standard.DockerRun, "AGENT_TOKEN=token-123")
	require.Contains(t, standard.DockerRun, "-v arcane-data:/app/data")
	require.Contains(t, standard.DockerCompose, "- arcane-data:/app/data")
	require.NotContains(t, standard.DockerRun, "-v arcane-data:/data")

	edgeSnippets, err := svc.GenerateEdgeDeploymentSnippets(t.Context(), "env-2", "https://manager.example.com", "token-456", nil)
	require.NoError(t, err)
	require.NotNil(t, edgeSnippets)
	require.NotContains(t, edgeSnippets.DockerRun, "EDGE_TRANSPORT=websocket")
	require.NotContains(t, edgeSnippets.DockerCompose, "EDGE_TRANSPORT=websocket")
	require.Contains(t, edgeSnippets.DockerRun, "EDGE_TRANSPORT=poll")
	require.Contains(t, edgeSnippets.DockerCompose, "EDGE_TRANSPORT=poll")
	require.Contains(t, edgeSnippets.DockerRun, "AGENT_TOKEN=token-456")
	require.Contains(t, edgeSnippets.DockerRun, "-v arcane-data:/app/data")
	require.Contains(t, edgeSnippets.DockerCompose, "- arcane-data:/app/data")
	require.NotContains(t, edgeSnippets.DockerRun, "-v arcane-data:/data")
}

func TestEnvironmentService_GenerateDeploymentSnippets_PublishesAgentURLPort(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name        string
		agentURL    string
		portMapping string
	}{
		{name: "default port", agentURL: "http://agent.example.com:3553", portMapping: "3553:3553"},
		{name: "custom port", agentURL: "http://agent.example.com:1234", portMapping: "1234:3553"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			svc := NewEnvironmentService(nil, nil, nil, nil, nil, nil)

			deploymentSnippets, err := svc.GenerateDeploymentSnippets(t.Context(), "env-1", "https://manager.example.com", tt.agentURL, "token-123")
			require.NoError(t, err)
			require.NotNil(t, deploymentSnippets)
			require.Contains(t, deploymentSnippets.DockerRun, "  -p "+tt.portMapping+" \\")
			require.Contains(t, deploymentSnippets.DockerCompose, fmt.Sprintf("    ports:\n      - %q\n", tt.portMapping))
			require.Contains(t, deploymentSnippets.DockerRun, "MANAGER_API_URL=https://manager.example.com")
			require.Contains(t, deploymentSnippets.DockerCompose, "MANAGER_API_URL=https://manager.example.com")
			if tt.portMapping != "3553:3553" {
				require.NotContains(t, deploymentSnippets.DockerRun, "-p 3553:3553")
				require.NotContains(t, deploymentSnippets.DockerCompose, "3553:3553")
			}
		})
	}
}

func TestAgentHostPortInternal(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "explicit custom port", url: "http://agent.example.com:1234", want: "1234"},
		{name: "default agent port", url: "http://agent.example.com:3553", want: "3553"},
		{name: "http without port uses 80", url: "http://agent.example.com", want: "80"},
		{name: "https without port uses 443", url: "https://agent.example.com", want: "443"},
		{name: "ipv6 with port", url: "http://[2001:db8::1]:9999", want: "9999"},
		{name: "empty falls back to container port", url: "", want: "3553"},
		{name: "invalid falls back to container port", url: "not a url", want: "3553"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tt.want, snippets.AgentHostPort(tt.url))
		})
	}
}

func TestEnvironmentService_ListEnvironmentsPaginated_FiltersByAccessibleEnvIDs(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	createTestEnvironment(t, db, "0", "http://localhost:3552", nil)
	createNamedTestEnvironmentInternal(t, db, "env-a", "Env A", "http://a.example", new("token-a"))
	createNamedTestEnvironmentInternal(t, db, "env-b", "Env B", "http://b.example", new("token-b"))

	newParams := func(typeFilter string) pagination.QueryParams {
		filters := map[string]string{}
		filters["type"] = cmp.Or(typeFilter, filters["type"])
		return pagination.QueryParams{
			Start: 0, Limit: 20,
			Filters: filters,
		}
	}

	// nil = no restriction: every environment is returned.
	all, _, err := svc.ListEnvironmentsPaginated(ctx, newParams(""), nil)
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"0", "env-a", "env-b"}, environmentIDsInternal(all))

	// A non-nil allow-list restricts the result on the DB path.
	scoped, _, err := svc.ListEnvironmentsPaginated(ctx, newParams(""), []string{"env-a"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"env-a"}, environmentIDsInternal(scoped))

	// The runtime-filter path (type filter) honors the allow-list too.
	scopedTyped, _, err := svc.ListEnvironmentsPaginated(ctx, newParams("http"), []string{"env-a"})
	require.NoError(t, err)
	require.ElementsMatch(t, []string{"env-a"}, environmentIDsInternal(scopedTyped))

	// An empty (non-nil) allow-list yields no environments on either path.
	none, _, err := svc.ListEnvironmentsPaginated(ctx, newParams(""), []string{})
	require.NoError(t, err)
	require.Empty(t, none)

	noneTyped, _, err := svc.ListEnvironmentsPaginated(ctx, newParams("http"), []string{})
	require.NoError(t, err)
	require.Empty(t, noneTyped)
}

func TestEnvironmentService_ListEnvironmentsPaginated_FiltersByRuntimeType(t *testing.T) {
	ctx := t.Context()
	db := setupEnvironmentServiceTestDB(t)
	svc := NewEnvironmentService(db, nil, nil, nil, nil, nil)

	now := time.Now()
	envs := []Environment{
		{
			ID: "0", CreatedAt: now, UpdatedAt: &now,
			Name:    "Local",
			ApiUrl:  "http://localhost:3552",
			Status:  string(EnvironmentStatusOnline),
			Enabled: true,
		},
		{
			ID: "env-edge", CreatedAt: now, UpdatedAt: &now,
			Name:    "Edge",
			ApiUrl:  "edge://agent",
			Status:  string(EnvironmentStatusOffline),
			Enabled: true,
			IsEdge:  true,
		},
		{
			ID: "env-grpc", CreatedAt: now, UpdatedAt: &now,
			Name:    "gRPC",
			ApiUrl:  "edge://grpc",
			Status:  string(EnvironmentStatusOffline),
			Enabled: true,
			IsEdge:  true,
		},
		{
			ID: "env-websocket", CreatedAt: now, UpdatedAt: &now,
			Name:    "WebSocket",
			ApiUrl:  "edge://websocket",
			Status:  string(EnvironmentStatusOffline),
			Enabled: true,
			IsEdge:  true,
		},
		{
			ID: "env-polling", CreatedAt: now, UpdatedAt: &now,
			Name:    "Polling",
			ApiUrl:  "edge://polling",
			Status:  string(EnvironmentStatusOffline),
			Enabled: true,
			IsEdge:  true,
		},
		{
			ID: "env-last-ws", CreatedAt: now, UpdatedAt: &now,
			Name:              "LastWebsocket",
			ApiUrl:            "edge://last-ws",
			Status:            string(EnvironmentStatusOffline),
			Enabled:           true,
			IsEdge:            true,
			LastEdgeTransport: new("websocket"),
		},
	}
	require.NoError(t, db.WithContext(ctx).Create(&envs).Error)

	edge.GetRegistry().Unregister("env-grpc")
	edge.GetRegistry().Unregister("env-websocket")
	edge.GetRegistry().Unregister("env-polling")
	t.Cleanup(func() {
		edge.GetRegistry().Unregister("env-grpc")
		edge.GetRegistry().Unregister("env-websocket")
		edge.GetRegistry().Unregister("env-polling")
	})

	grpcTunnel := edge.NewAgentTunnelWithConn("env-grpc", edge.NewGRPCManagerTunnelConn(nil))
	edge.GetRegistry().Register("env-grpc", grpcTunnel)

	websocketTunnel, closeWebSocketTunnel := newTestWebSocketTunnelInternal(t, "env-websocket")
	defer closeWebSocketTunnel()
	edge.GetRegistry().Register("env-websocket", websocketTunnel)

	edge.GetPollRuntimeRegistry().Update("env-polling", edge.DefaultTunnelPollInterval, time.Now())

	tests := []struct {
		name       string
		typeFilter string
		wantIDs    []string
	}{
		{name: "http", typeFilter: "http", wantIDs: []string{"0"}},
		// Poll-only edge environments (no tunnel transport) classify as plain edge.
		{name: "edge", typeFilter: "edge", wantIDs: []string{"env-edge", "env-polling"}},
		{name: "grpc", typeFilter: "grpc", wantIDs: []string{"env-grpc"}},
		// Disconnected agents classify by the transport they last used.
		{name: "websocket", typeFilter: "websocket", wantIDs: []string{"env-websocket", "env-last-ws"}},
		{name: "multiple", typeFilter: "http,grpc", wantIDs: []string{"0", "env-grpc"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			listedEnvironments, _, err := svc.ListEnvironmentsPaginated(ctx, pagination.QueryParams{
				Start: 0, Limit: 20,
				Filters: map[string]string{"type": tt.typeFilter},
			}, nil)
			require.NoError(t, err)
			require.ElementsMatch(t, tt.wantIDs, environmentIDsInternal(listedEnvironments))
		})
	}
}

func environmentIDsInternal(environments []environment.Environment) []string {
	ids := make([]string, 0, len(environments))
	for _, env := range environments {
		ids = append(ids, env.ID)
	}
	return ids
}

func newTestWebSocketTunnelInternal(t *testing.T, envID string) (*edge.AgentTunnel, func()) {
	t.Helper()

	connCh := make(chan *websocket.Conn, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if serveCompatibleAgentCheck(t, w, r) {
			return
		}
		conn, err := websocket.Accept(w, r, nil)
		if !assert.NoError(t, err) {
			return
		}
		connCh <- conn
	}))

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http")
	clientConn, _, err := websocket.Dial(t.Context(), wsURL, nil)
	require.NoError(t, err)

	serverConn := <-connCh
	return edge.NewAgentTunnelWithConn(envID, edge.NewTunnelConn(serverConn)), func() {
		_ = clientConn.CloseNow()
		server.Close()
	}
}
