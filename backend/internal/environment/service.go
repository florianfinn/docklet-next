// Source: getarcaneapp/arcane/backend/internal/environment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations; transactional host edits and credential revision fencing.; hub-owned one-shot restart wiring; the generic update never writes the host status.

package environment

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/health"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/proxy"
	"github.com/florianfinn/docklet-next/backend/internal/environment/children/snippets"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/hoststate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/entityjobs"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/validation"
	"github.com/florianfinn/docklet-next/types/containerregistry"
	"github.com/samber/hot"
	"go.getarcane.app/kit/normalization"
	"go.getarcane.app/sys/crypto"
	"gorm.io/gorm"
	"log/slog"
	"net/http"
	"strings"
	"sync/atomic"
	"time"
	"uuid"
)

type EnvironmentService struct {
	restarts         *restartController
	runtimeRemoval   RuntimeRemoval
	snapshots        hoststate.Store
	agentRuntime     agentRuntime
	db               *database.DB
	httpClient       *http.Client
	dockerService    *docker.DockerClientService
	eventService     *event.EventService
	settingsService  *settings.SettingsService
	edgeTokens       *edgeTokenCacheInternal
	remoteEnvs       *remoteEnvSnapshotCacheInternal
	environmentCache *hot.HotCache[environmentCacheKeyInternal, Environment]
	environmentGens  [environmentCacheStripes]atomic.Uint64

	// jobs carries the scheduler and app lifecycle context, injected
	// post-construction via SetScheduler (manager-only). Each enabled environment
	// gets its own health-check job; this replaces the single global
	// environment-health job.
	jobs *entityjobs.Registry

	// runtimeWatchers receive a coalesced wake-up whenever an environment's
	// liveness changes. See environment_runtime_notify.go.
	runtimeWatchers runtimeWatchersInternal

	// syncGate skips the periodic registry pushes to an agent
	// while the payload it last accepted is unchanged.
	syncGate utils.SyncGate

	proxy    *proxy.Service
	health   *health.Service
	snippets *snippets.Service
	apiKeys  *apikey.ApiKeyService
}

const (
	// LocalEnvironmentID is the reserved ID of the environment Arcane manages directly.
	LocalEnvironmentID                   = "0"
	localEnvironmentFallbackNameInternal = "Local"

	// SyncDeliveryExpiry bounds how long an accepted config push is trusted. An
	// agent rebuilt with a fresh data volume while its status stayed online gets
	// everything again within this window without operator action.
	SyncDeliveryExpiry = time.Hour
)

var (
	ErrEnvironmentAccessTokenRequired = errors.New("environment access token required")
	ErrInvalidEnvironmentAccessToken  = remenv.ErrAgentInvalidToken
	ErrEnvironmentNotFound            = errors.New("environment not found")
)

// ForgetSyncState makes the next sync of every resource group resend to the
// environment regardless of whether the payload changed.
func (s *EnvironmentService) ForgetSyncState(environmentID string) {
	s.syncGate.Forget(environmentID)
}

func NewEnvironmentService(
	db *database.DB,
	httpClient *http.Client,
	dockerService *docker.DockerClientService,
	eventService *event.EventService,
	settingsService *settings.SettingsService,
	apiKeyService *apikey.ApiKeyService,
) *EnvironmentService {
	if httpClient == nil {
		httpClient = http.DefaultClient
	}
	s := &EnvironmentService{
		syncGate:         utils.SyncGate{Expiry: SyncDeliveryExpiry},
		db:               db,
		httpClient:       httpClient,
		dockerService:    dockerService,
		eventService:     eventService,
		settingsService:  settingsService,
		edgeTokens:       newEdgeTokenCacheInternal(),
		remoteEnvs:       newRemoteEnvSnapshotCacheInternal(),
		environmentCache: newEnvironmentCacheInternal(),
		jobs:             entityjobs.New(environmentHealthJobPrefix, environmentHealthAdmissionScopeInternal),
		health:           health.New(httpClient, dockerService),
		snippets:         snippets.New(eventService),
		apiKeys:          apiKeyService,
	}
	s.proxy = proxy.New(db, httpClient, settingsService, &s.syncGate)
	s.proxy.SetAccessCheck(s.AgentAccess)
	return s
}

func (s *EnvironmentService) ResolveEdgeEnvironmentByToken(ctx context.Context, token string) (string, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return "", errors.New("agent token required")
	}

	var env Environment
	if err := s.db.WithContext(ctx).
		Select("id", "access_token").
		Where("enabled = ?", true).
		Where("is_edge = ?", true).
		Where("access_token = ?", token).
		First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			s.logEdgeTokenResolveMissInternal(ctx, token)
			return "", errors.New("invalid agent token")
		}
		return "", fmt.Errorf("failed to resolve edge environment by token: %w", err)
	}

	s.edgeTokens.put(env.ID, token)
	return env.ID, nil
}

// logEdgeTokenResolveMissInternal emits a debug log diagnosing why an agent
// token failed to resolve to an edge environment. Counts existing edge
// environments (by access_token presence) so operators can distinguish
// "no edge envs configured" from "token does not match any configured env".
// Token contents are never logged in full — only length and a short
// fingerprint that cannot be reversed.
func (s *EnvironmentService) logEdgeTokenResolveMissInternal(ctx context.Context, token string) {
	if s == nil || s.db == nil {
		return
	}
	if !slog.Default().Enabled(ctx, slog.LevelDebug) {
		return
	}

	var totalEdgeEnvs int64
	var edgeEnvsWithToken int64
	totalEdgeEnvsErr := s.db.WithContext(ctx).Model(&Environment{}).Where("is_edge = ?", true).Count(&totalEdgeEnvs).Error
	edgeEnvsWithTokenErr := s.db.WithContext(ctx).Model(&Environment{}).
		Where("is_edge = ?", true).
		Where("access_token IS NOT NULL AND access_token != ?", "").
		Count(&edgeEnvsWithToken).Error

	args := []any{
		"token_length", len(token),
		"token_fingerprint", remenv.RedactedTokenFingerprint(token),
	}
	if totalEdgeEnvsErr == nil {
		args = append(args, "edge_envs_total", totalEdgeEnvs)
	}
	if edgeEnvsWithTokenErr == nil {
		args = append(args, "edge_envs_with_access_token", edgeEnvsWithToken)
	}

	slog.DebugContext(ctx, "Edge agent token did not match any environment", args...)
}

// ResolveEnvironmentName looks up an environment and returns the label to show for it.
// Use this instead of hardcoding a name for a known ID: names are user-editable, so
// even the local environment's is not fixed.
func (s *EnvironmentService) ResolveEnvironmentName(ctx context.Context, environmentID string) string {
	if strings.TrimSpace(environmentID) == "" {
		environmentID = LocalEnvironmentID
	}
	env, err := s.GetEnvironmentByIDCached(ctx, environmentID)
	if err != nil || env == nil {
		if !errors.Is(err, context.Canceled) {
			slog.WarnContext(ctx, "failed to resolve environment name", "environmentId", environmentID, "error", err)
		}
		return DisplayName(environmentID, "")
	}
	return DisplayName(env.ID, env.Name)
}

func (s *EnvironmentService) EnsureLocalEnvironment(ctx context.Context, appUrl string) error {
	var existingEnv Environment
	err := s.db.WithContext(ctx).Where("id = ?", LocalEnvironmentID).First(&existingEnv).Error

	if err == nil {
		// Local environment already exists, ensure ApiUrl matches current appUrl
		if existingEnv.ApiUrl != appUrl {
			if updateLocalURLErr := s.db.WithContext(ctx).Model(&existingEnv).Updates(map[string]any{"api_url": appUrl, "credential_revision": gorm.Expr("credential_revision + 1")}).Error; updateLocalURLErr != nil {
				return fmt.Errorf("failed to update local environment api url: %w", updateLocalURLErr)
			}
			s.invalidateEnvironmentCacheInternal(LocalEnvironmentID)
			slog.InfoContext(ctx, "updated local environment api url", "id", LocalEnvironmentID, "url", appUrl)
		}
		return nil
	}

	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("failed to check for local environment: %w", err)
	}

	// Create the local environment
	now := time.Now()
	localEnv := &Environment{
		ID:        LocalEnvironmentID,
		CreatedAt: now,
		UpdatedAt: new(now),
		Name:      "Local Docker",
		ApiUrl:    appUrl,
		Status:    string(EnvironmentStatusOnline),
		Enabled:   true,
	}

	if createLocalEnvironmentErr := s.db.WithContext(ctx).Create(localEnv).Error; createLocalEnvironmentErr != nil {
		return fmt.Errorf("failed to create local environment: %w", createLocalEnvironmentErr)
	}
	s.invalidateEnvironmentCacheInternal(LocalEnvironmentID)

	slog.InfoContext(ctx, "created local environment record", "id", LocalEnvironmentID)
	return nil
}

func (s *EnvironmentService) CreateEnvironment(ctx context.Context, env *Environment, userID, username *string) (*Environment, error) {
	if err := normalization.Normalize(env); err != nil {
		return nil, err
	}
	env.ID = uuid.New().String()

	// Only set status to offline if not already set (e.g., API key flow sets it to pending)
	if env.Status == "" {
		env.Status = string(EnvironmentStatusOffline)
	}

	now := time.Now()
	env.CreatedAt = now
	env.UpdatedAt = new(now)

	if err := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := validateHostName(tx, env.ID, env.Name); err != nil {
			return err
		}
		return tx.Create(env).Error
	}); err != nil {
		return nil, fmt.Errorf("failed to create environment: %w", err)
	}

	// Create event in background
	go s.createEnvironmentEvent(
		context.WithoutCancel(
			ctx,
		),
		env.ID,
		env.Name,
		event.EventTypeEnvironmentCreate,
		"Environment Created",
		fmt.Sprintf(
			"Environment '%s' was created",
			env.Name,
		),
		event.EventSeveritySuccess,
		userID,
		username,
	)

	if env.Enabled {
		s.registerHealthJobInternal(ctx, env.ID)
	}
	s.remoteEnvs.put(*env)
	s.invalidateEnvironmentCacheInternal(env.ID)
	s.NotifyRuntimeStateChanged()

	return env, nil
}

func (s *EnvironmentService) GetEnvironmentByID(ctx context.Context, id string) (*Environment, error) {
	var envRecord Environment
	if err := s.db.WithContext(ctx).Where("id = ?", id).First(&envRecord).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrEnvironmentNotFound
		}
		return nil, fmt.Errorf("failed to get environment: %w", err)
	}
	return &envRecord, nil
}

func (s *EnvironmentService) UpdateEnvironment(ctx context.Context, id string, updates map[string]any, userID, username *string) (*Environment, error) {
	if _, ok := updates["status"]; ok {
		return nil, errors.New("host status is owned by its status writers")
	}
	if name, ok := updates["name"].(string); ok {
		updates["name"] = normalization.Text(name, true, true)
	}
	current, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return nil, err
	}

	var nextAPIURL *string
	if rawAPIURL, ok := updates["api_url"]; ok {
		if apiURL, isString := rawAPIURL.(string); isString {
			nextAPIURL = new(apiURL)
		}
	}
	_, accessTokenUpdated := updates["access_token"]
	if validateCredentialTargetChangeErr := validation.ValidateCredentialTargetChange(
		"environment API URL",
		current.ApiUrl,
		nextAPIURL,
		func(value string) string {
			normalized, normalizeErr := httpx.NormalizeBaseURL(value)
			if normalizeErr != nil {
				return strings.TrimSpace(value)
			}
			return normalized
		},
		map[string]bool{"accessToken": current.AccessToken != nil && *current.AccessToken != ""},
		map[string]bool{"accessToken": accessTokenUpdated},
	); validateCredentialTargetChangeErr != nil {
		return nil, validateCredentialTargetChangeErr
	}

	updates["updated_at"] = new(time.Now())

	fenceCredentialUpdate(updates)
	if updateEnvironmentErr := s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if name, ok := updates["name"].(string); ok {
			if err := validateHostName(tx, id, name); err != nil {
				return err
			}
		}
		return tx.Model(&Environment{}).Where("id = ?", id).Updates(updates).Error
	}); updateEnvironmentErr != nil {
		return nil, fmt.Errorf("failed to update environment: %w", updateEnvironmentErr)
	}
	s.invalidateEnvironmentCacheInternal(id)
	s.NotifyRuntimeStateChanged()

	updated, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return nil, err
	}
	s.remoteEnvs.put(*updated)
	if _, endpointChanged := updates["api_url"]; endpointChanged {
		s.revokeAgentSessions(id)
	}
	if enabled, changed := updates["enabled"].(bool); changed && !enabled {
		s.revokeAgentSessions(id)
	}

	if rawAccessToken, ok := updates["access_token"]; ok {
		accessToken, _ := rawAccessToken.(string)
		s.revokeAgentSessions(id)
		s.edgeTokens.sync(id, accessToken)
	}

	// Reconcile the per-environment health job when the enabled flag is toggled.
	if rawEnabled, ok := updates["enabled"]; ok {
		if enabled, isBool := rawEnabled.(bool); isBool {
			if enabled {
				s.registerHealthJobInternal(ctx, id)
			} else {
				s.removeHealthJobInternal(ctx, id)
			}
		}
	}

	// Create event in background (skip for local environment)
	if id != "0" {
		go s.createEnvironmentEvent(
			context.WithoutCancel(
				ctx,
			),
			id,
			updated.Name,
			event.EventTypeEnvironmentUpdate,
			"Environment Updated",
			fmt.Sprintf(
				"Environment '%s' was updated",
				updated.Name,
			),
			event.EventSeverityInfo,
			userID,
			username,
		)
	}

	return updated, nil
}

func (s *EnvironmentService) DeleteEnvironment(ctx context.Context, id string, userID, username *string) error {
	// Get environment details before deletion
	env, err := s.GetEnvironmentByID(ctx, id)
	if err != nil {
		return err
	}

	// Stop the per-environment health job before the row is removed.
	s.removeHealthJobInternal(ctx, id)

	remove := func(tx *gorm.DB) error { return tx.Delete(&Environment{}, "id = ?", id).Error }
	var deleteEnvironmentErr error
	if s.runtimeRemoval != nil {
		deleteEnvironmentErr = s.runtimeRemoval(ctx, id, remove)
	} else {
		deleteEnvironmentErr = remove(s.db.WithContext(ctx))
	}
	if deleteEnvironmentErr != nil {
		if env.Enabled {
			s.registerHealthJobInternal(ctx, env.ID)
		}
		return fmt.Errorf("failed to delete environment: %w", deleteEnvironmentErr)
	}

	s.revokeAgentSessions(id)
	s.edgeTokens.invalidate(id)
	s.ForgetSyncState(id)
	s.remoteEnvs.remove(id)
	s.snapshots.Forget(id)
	s.invalidateEnvironmentCacheInternal(id)
	s.NotifyRuntimeStateChanged()

	// Create event in background
	go s.createEnvironmentEvent(
		context.WithoutCancel(
			ctx,
		),
		id,
		env.Name,
		event.EventTypeEnvironmentDelete,
		"Environment Deleted",
		fmt.Sprintf(
			"Environment '%s' was deleted",
			env.Name,
		),
		event.EventSeverityWarning,
		userID,
		username,
	)

	return nil
}

func (
	s *EnvironmentService,
) createEnvironmentEvent(
	ctx context.Context,
	envID, envName string,
	eventType event.EventType,
	title, description string,
	severity event.EventSeverity,
	userID, username *string,
) {
	if s == nil || s.eventService == nil {
		return
	}

	_, _ = s.eventService.CreateEvent(ctx, event.CreateEventRequest{
		Type:          eventType,
		Severity:      severity,
		Title:         title,
		Description:   description,
		ResourceType:  new("environment"),
		ResourceID:    new(envID),
		ResourceName:  new(envName),
		UserID:        userID,
		Username:      username,
		EnvironmentID: new(envID),
	})
}

// RegenerateEnvironmentApiKey rotates the key and updates its environment binding.
func (s *EnvironmentService) RegenerateEnvironmentApiKey(ctx context.Context, env *Environment, userID, username string) (string, error) {
	if env == nil {
		return "", errors.New("environment is required")
	}
	if s.apiKeys == nil {
		return "", errors.New("api key service not configured")
	}
	created, err := s.apiKeys.CreateEnvironmentApiKey(ctx, env.ID)
	if err != nil {
		return "", fmt.Errorf("failed to create environment API key: %w", err)
	}
	if err := s.linkEnvironmentApiKey(ctx, env.ID, created.ID, created.Key, userID, username, env.Name); err != nil {
		if deleteErr := s.apiKeys.DeleteApiKey(ctx, created.ID); deleteErr != nil && !errors.Is(deleteErr, apikey.ErrApiKeyNotFound) {
			slog.ErrorContext(ctx, "Failed to clean up unlinked environment API key", "environmentId", env.ID, "error", deleteErr)
		}
		return "", err
	}
	if env.ApiKeyID != nil && *env.ApiKeyID != created.ID {
		if err := s.apiKeys.DeleteApiKey(ctx, *env.ApiKeyID); err != nil && !errors.Is(err, apikey.ErrApiKeyNotFound) {
			slog.ErrorContext(ctx, "Failed to delete previous environment API key; the old key remains valid until deleted manually", "environmentId", env.ID, "error", err)
		}
	}
	return created.Key, nil
}

func (s *EnvironmentService) linkEnvironmentApiKey(ctx context.Context, envID, newApiKeyID, apiKey, userID, username, envName string) error {
	// Trim once at the boundary so the value persisted, the value cached,
	// and the value returned by callers (which already TrimSpace before
	// returning) all stay byte-identical. Any divergence here would surface
	// as a 401 "invalid agent token" because lookup is direct equality.
	apiKey = strings.TrimSpace(apiKey)

	updates := map[string]any{
		"api_key_id":   newApiKeyID,
		"access_token": apiKey,
		"status":       string(EnvironmentStatusPending),
	}

	fenceCredentialUpdate(updates)
	result := s.db.WithContext(ctx).Model(&Environment{}).Where("id = ?", envID).Updates(updates)
	if result.Error != nil {
		return fmt.Errorf("failed to update environment with new API key: %w", result.Error)
	}
	if result.RowsAffected == 0 {
		// A zero-row update would otherwise report a successful rotation while
		// the new key was never linked to anything.
		return ErrEnvironmentNotFound
	}
	s.invalidateEnvironmentCacheInternal(envID)
	s.NotifyRuntimeStateChanged()

	s.revokeAgentSessionsInternal(envID, true)
	s.edgeTokens.sync(envID, apiKey)
	now := time.Now()
	s.remoteEnvs.update(envID, func(env *Environment) {
		env.ApiKeyID = &newApiKeyID
		env.AccessToken = &apiKey
		env.Status = string(EnvironmentStatusPending)
		env.UpdatedAt = &now
	})

	// Create event log in background
	go s.createEnvironmentEvent(
		context.WithoutCancel(
			ctx,
		),
		envID,
		envName,
		event.EventTypeEnvironmentApiKeyRegenerated,
		"API Key Regenerated",
		"Environment API key was regenerated and status set to pending",
		event.EventSeverityInfo,
		new(
			userID,
		),
		new(
			username,
		),
	)

	return nil
}

func (s *EnvironmentService) GetDB() *database.DB {
	return s.db
}

func (s *EnvironmentService) ResolveEnvironmentByAccessToken(ctx context.Context, token string) (*Environment, error) {
	token = strings.TrimSpace(token)
	if token == "" {
		return nil, ErrEnvironmentAccessTokenRequired
	}

	var env Environment
	if err := s.db.WithContext(ctx).
		Where("access_token = ?", token).
		First(&env).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil, ErrInvalidEnvironmentAccessToken
		}
		return nil, fmt.Errorf("failed to resolve environment by access token: %w", err)
	}

	return &env, nil
}

func (s *EnvironmentService) GetEnabledRegistryCredentials(ctx context.Context) ([]containerregistry.Credential, error) {
	var registries []registry.ContainerRegistry
	if err := s.db.WithContext(ctx).Where("enabled = ?", true).Find(&registries).Error; err != nil {
		return nil, fmt.Errorf("failed to get enabled container registries: %w", err)
	}

	var creds []containerregistry.Credential
	for _, reg := range registries {
		if !reg.Enabled || reg.Username == "" || reg.Token == "" {
			continue
		}

		decryptedToken, err := crypto.Decrypt(reg.Token)
		if err != nil {
			slog.WarnContext(ctx, "Failed to decrypt registry token", "registryUrl", reg.URL, "error", err.Error())
			continue
		}

		creds = append(creds, containerregistry.Credential{
			URL:      reg.URL,
			Username: reg.Username,
			Token:    decryptedToken,
			Enabled:  reg.Enabled,
		})
	}

	return creds, nil
}

// ReadSnapshots shares successful inventory reads with the owning read adapters.
func (s *EnvironmentService) ReadSnapshots() *hoststate.Store { return &s.snapshots }

// InventoryPending keeps unpaired credentials off the ordinary inventory path.
func (s *EnvironmentService) InventoryPending(ctx context.Context, id string) (bool, error) {
	env, err := s.GetEnvironmentByIDCached(ctx, id)
	if err != nil {
		return false, err
	}
	return env.Status == string(EnvironmentStatusPending), nil
}
