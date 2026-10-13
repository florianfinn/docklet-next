// Source: getarcaneapp/arcane/backend/internal/environment/children/proxy/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

// Package proxy owns the transport to remote environment agents: request
// building, edge tunnel routing, JSON decoding and configuration pushes.
package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/florianfinn/docklet-next/types/containerregistry"
	"go.getarcane.app/sys/crypto"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/registry"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/timeouts"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/httpx"
)

// Service sends requests to remote environment agents over HTTP or edge
// tunnels and pushes manager-owned configuration to them.
type Service struct {
	db              *database.DB
	client          *remenv.Client
	settingsService *settings.SettingsService
	syncGate        *utils.SyncGate
}

// New builds the remote transport. syncGate is owned by the parent so it can
// forget delivery state when an environment reconnects.
func New(db *database.DB, httpClient *http.Client, settingsService *settings.SettingsService, syncGate *utils.SyncGate) *Service {
	return &Service{
		db: db,
		client: remenv.NewClient(httpClient, remenv.TunnelTransportFuncs{
			EnsureAvailableFunc: ensureTunnelAvailableInternal,
			DoFunc:              doTunnelRequestInternal,
		}),
		settingsService: settingsService,
		syncGate:        syncGate,
	}
}

// Target is a validated remote environment endpoint; build it with NewTarget.
type Target struct {
	id          string
	name        string
	isEdge      bool
	accessToken *string
	targetURL   string
}

// NewTarget validates a remote environment's endpoint. The local environment
// is never a proxy target.
func NewTarget(id, name, apiURL string, isEdge bool, accessToken *string) (*Target, error) {
	if id == "0" {
		return nil, errors.New("cannot proxy request to local environment")
	}

	targetURL := strings.TrimRight(apiURL, "/")
	if !isEdge {
		validatedTargetURL, err := httpx.NormalizeBaseURL(apiURL)
		if err != nil {
			return nil, fmt.Errorf("invalid environment API URL: %w", err)
		}
		targetURL = validatedTargetURL
	}

	return &Target{
		id:          id,
		name:        name,
		isEdge:      isEdge,
		accessToken: accessToken,
		targetURL:   targetURL,
	}, nil
}

// Context bounds a proxied request by the configured proxy timeout.
func (s *Service) Context(ctx context.Context) (context.Context, context.CancelFunc) {
	if s != nil && s.settingsService != nil {
		cfg := s.settingsService.GetSettingsConfig()
		return context.WithTimeout(ctx, timeouts.GetDuration(cfg.ProxyRequestTimeout.AsInt(), timeouts.DefaultProxyRequest))
	}

	return context.WithTimeout(ctx, timeouts.DefaultProxyRequest)
}

func buildRemoteRequestInternal(
	target *Target,
	method string,
	path string,
	body []byte,
	headers map[string]string,
) (remenv.Request, error) {
	if target == nil {
		return remenv.Request{}, errors.New("remote environment target is required")
	}

	requestHeaders := make(map[string]string, len(headers)+2)
	maps.Copy(requestHeaders, headers)
	if len(body) > 0 && method != http.MethodGet && requestHeaders["Content-Type"] == "" {
		requestHeaders["Content-Type"] = "application/json"
	}
	remenv.ApplyAgentTokenHeaderMap(requestHeaders, target.accessToken)

	return remenv.Request{
		EnvironmentID: target.id,
		IsEdge:        target.isEdge,
		Method:        method,
		URL:           target.targetURL + path,
		Path:          path,
		Headers:       requestHeaders,
		Body:          body,
	}, nil
}

// Execute sends one request to the target agent.
func (s *Service) Execute(
	ctx context.Context,
	target *Target,
	method string,
	path string,
	body []byte,
) (*remenv.Response, error) {
	// Forward the activity batch ID so bulk actions proxied to a remote
	// environment group the same way they do locally.
	var headers map[string]string
	if batchID := utils.ActivityBatchIDFromContext(ctx); batchID != "" {
		headers = map[string]string{middleware.HeaderActivityBatchID: batchID}
	}
	request, err := buildRemoteRequestInternal(target, method, path, body, headers)
	if err != nil {
		return nil, err
	}

	resp, err := s.client.Do(ctx, request)
	if err != nil {
		return nil, fmt.Errorf("failed to send request to environment %s: %w", target.name, err)
	}

	return resp, nil
}

// JSON sends one request and decodes a successful JSON response into out.
func (s *Service) JSON(
	ctx context.Context,
	target *Target,
	method string,
	path string,
	body []byte,
	out any,
) error {
	resp, err := s.Execute(ctx, target, method, path, body)
	if err != nil {
		return err
	}
	if requireSuccessErr := resp.RequireSuccess(); requireSuccessErr != nil {
		return requireSuccessErr
	}
	// This is the erased decode behind the RemoteJSONProxy func-value seam;
	// typed callers go through RemoteJSONProxy.JSON or remenv's generic decode.
	if out == nil {
		return nil
	}
	if unmarshalErr := json.Unmarshal(resp.Body, out); unmarshalErr != nil {
		return &remenv.DecodeError{Err: unmarshalErr}
	}

	return nil
}

// SyncRegistries syncs all registries from this manager to a remote environment
func (s *Service) SyncRegistries(ctx context.Context, target *Target) error {
	return s.fanOutSyncToEnvironment(ctx, target, "registries", "/api/container-registries/sync",
		func(reg registry.ContainerRegistry) (containerregistry.Sync, bool, error) {
			registryType, typeErr := registry.NormalizeRegistryType(reg.RegistryType)
			if typeErr != nil {
				return containerregistry.Sync{}, false, fmt.Errorf("normalize registry type for sync %s: %w", reg.ID, typeErr)
			}

			syncItem := containerregistry.Sync{
				ID:              reg.ID,
				URL:             reg.URL,
				Description:     reg.Description,
				Insecure:        reg.Insecure,
				Enabled:         reg.Enabled,
				RegistryType:    registryType,
				RepositoryNames: reg.RepositoryNames,
				CreatedAt:       reg.CreatedAt,
				UpdatedAt:       reg.UpdatedAt,
			}

			if registryType == registry.RegistryTypeECR {
				decryptedSecret, err := crypto.Decrypt(reg.AWSSecretAccessKey)
				if err != nil {
					return containerregistry.Sync{}, false, fmt.Errorf("failed to decrypt ECR secret for registry %s for sync: %w", reg.ID, err)
				}

				syncItem.AWSAccessKeyID = reg.AWSAccessKeyID
				syncItem.AWSSecretAccessKey = decryptedSecret
				syncItem.AWSRegion = reg.AWSRegion
			} else {
				decryptedToken, err := crypto.Decrypt(reg.Token)
				if err != nil {
					return containerregistry.Sync{}, false, fmt.Errorf("failed to decrypt token for registry %s for sync: %w", reg.ID, err)
				}

				syncItem.Username = reg.Username
				syncItem.Token = decryptedToken
			}

			return syncItem, true, nil
		},
		func(items []containerregistry.Sync) containerregistry.SyncRequest {
			return containerregistry.SyncRequest{Registries: items}
		},
	)
}

func (s *Service) fanOutSyncToEnvironment[Model, Item, Request any](
	ctx context.Context,
	target *Target,
	kind string,
	path string,
	toSyncItem func(Model) (Item, bool, error),
	wrap func([]Item) Request,
) error {
	environmentID := target.id
	var records []Model
	if loadRecordsErr := s.db.WithContext(ctx).Find(&records).Error; loadRecordsErr != nil {
		return fmt.Errorf("failed to get %s: %w", kind, loadRecordsErr)
	}

	syncItems := make([]Item, 0, len(records))
	for _, record := range records {
		item, keep, toSyncItemErr := toSyncItem(record)
		if toSyncItemErr != nil {
			return toSyncItemErr
		}
		if keep {
			syncItems = append(syncItems, item)
		}
	}

	reqBody, err := json.Marshal(wrap(syncItems))
	if err != nil {
		return fmt.Errorf("failed to marshal sync request: %w", err)
	}
	unchanged, finishDelivery, err := s.syncGate.Begin(ctx, environmentID, path, reqBody)
	if err != nil {
		return fmt.Errorf("sync cancelled while waiting for an in-flight delivery: %w", err)
	}
	if unchanged {
		slog.DebugContext(ctx, "Skipping sync; payload unchanged since last delivery", "kind", kind, "environmentId", environmentID, "environmentName", target.name)
		return nil
	}
	delivered := false
	defer func() { finishDelivery(delivered) }()

	slog.InfoContext(ctx, "Starting sync to environment", "kind", kind, "environmentId", environmentID, "environmentName", target.name, "apiUrl", target.targetURL)

	reqCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()

	slog.InfoContext(ctx, "Sending sync request to agent", "kind", kind, "url", target.targetURL+path, "count", len(syncItems), "isEdge", target.isEdge)

	var result struct {
		Success bool `json:"success"`
		Data    struct {
			Message string `json:"message"`
		} `json:"data"`
	}
	if proxyJSONRequestForTargetErr := s.JSON(reqCtx, target, http.MethodPost, path, reqBody, &result); proxyJSONRequestForTargetErr != nil {
		return fmt.Errorf("failed to send sync request: %w", proxyJSONRequestForTargetErr)
	}

	if !result.Success {
		return fmt.Errorf("sync failed: %s", result.Data.Message)
	}
	delivered = true

	slog.InfoContext(ctx, "Successfully synced to environment", "kind", kind, "environmentId", environmentID, "environmentName", target.name)

	return nil
}

func doTunnelRequestInternal(
	ctx context.Context,
	envID string,
	method string,
	path string,
	headers map[string]string,
	body []byte,
) (*remenv.Response, error) {
	tunnel, ok := edge.GetRegistry().Get(envID).Get()
	if !ok {
		return nil, fmt.Errorf("no active tunnel for environment %s: %w", envID, remenv.ErrEnvironmentUnavailable)
	}
	if tunnel.Conn.IsClosed() {
		return nil, fmt.Errorf("tunnel for environment %s is closed: %w", envID, remenv.ErrEnvironmentUnavailable)
	}

	statusCode, respHeaders, respBody, err := edge.ProxyRequest(ctx, tunnel, method, path, "", headers, body)
	if err != nil {
		if errors.Is(err, edge.ErrTunnelConnectionClosed) {
			err = errors.Join(remenv.ErrEnvironmentUnavailable, err)
		}
		return nil, fmt.Errorf("tunnel request failed: %w", err)
	}

	return &remenv.Response{
		StatusCode: statusCode,
		Body:       respBody,
		Headers:    respHeaders,
	}, nil
}

func ensureTunnelAvailableInternal(ctx context.Context, envID string) error {
	if edge.HasActiveTunnel(envID) {
		return nil
	}

	if _, ok := edge.RequestTunnelAndWait(ctx, envID, edge.DefaultTunnelDemandTTL, edge.DefaultTunnelAcquireTimeout()).Get(); ok {
		return nil
	}

	return remenv.ErrEnvironmentUnavailable
}
