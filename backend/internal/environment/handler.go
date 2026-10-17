// Source: getarcaneapp/arcane/backend/internal/environment/handler.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed GitOps and saved Git repositories; functional cut: removed S3 backup destinations.

package environment

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/apikey"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/environment"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/streams/agg"
	"log/slog"
	"maps"
	"slices"
	"sync"
	"time"
)

const (
	localDockerEnvironmentID = "0"

	// Re-applies in-memory runtime state to cached rows, covering poll-mode TTL
	// expiry; CRUD, tunnel and health-check changes arrive on the runtime-change signal.
	environmentStreamRuntimeInterval = 5 * time.Second
	// Safety net for writes that bypass the runtime-change signal.
	environmentStreamReloadInterval = 60 * time.Second
	environmentStreamRefreshFloor   = 30 * time.Second
	environmentStreamHubKeyInternal = "environments"
)

// EnvironmentHandler handles environment management endpoints.
type EnvironmentHandler struct {
	background         sync.WaitGroup
	environmentService *EnvironmentService
	settingsService    *settings.SettingsService
	apiKeyService      *apikey.ApiKeyService
	eventService       *event.EventService
	cfg                *config.Config
	streamHub          *agg.Hub[[]environment.Environment]
	activityService    activity.Service
	appCtx             context.Context
}

type ListEnvironmentsInput struct {
	Search string `query:"search" doc:"Search query for filtering by name or API URL"`
	Sort   string `query:"sort" doc:"Column to sort by"`
	Order  string `query:"order" default:"asc" doc:"Sort direction (asc or desc)"`
	Start  int    `query:"start" default:"0" doc:"Start index for pagination"`
	Limit  int    `query:"limit" default:"20" doc:"Items per page"`
	Type   string `query:"type" doc:"Filter by environment type (comma-separated: http,edge,websocket,grpc,polling)"`
}

type CreateEnvironmentInput struct {
	Body environment.Create
}

type EnvironmentWithApiKey struct {
	environment.Environment

	ApiKey *string `json:"apiKey,omitempty" doc:"API key for pairing (only shown once during creation)"`
}

type GetEnvironmentInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type UpdateEnvironmentInput struct {
	ID   string `path:"id" doc:"Environment ID"`
	Body environment.Update
}

type DeleteEnvironmentInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type TestConnectionInput struct {
	ID   string                             `path:"id" doc:"Environment ID"`
	Body *environment.TestConnectionRequest `json:"body,omitempty"`
}

type UpdateHeartbeatInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type PairAgentInput struct {
	ID   string                        `path:"id" doc:"Environment ID (must be 0 for local)"`
	Body *environment.AgentPairRequest `json:"body,omitempty"`
}

type SyncEnvironmentInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type PairEnvironmentInput struct {
	XAPIKey string `header:"X-API-Key" doc:"API key for environment pairing"`
}

type DeploymentSnippet struct {
	DockerRun     string                 `json:"dockerRun" doc:"Docker run command snippet"`
	DockerCompose string                 `json:"dockerCompose" doc:"Docker compose YAML snippet"`
	MTLS          *DeploymentSnippetMTLS `json:"mtls,omitempty" doc:"Optional Arcane-generated mTLS deployment assets for edge agents"`
}

type GetDeploymentSnippetsInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type GetEnvironmentVersionInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type DownloadEdgeMTLSCAInput struct{}

type DownloadEnvironmentMTLSBundleInput struct {
	ID string `path:"id" doc:"Environment ID"`
}

type DownloadEnvironmentMTLSFileInput struct {
	ID       string `path:"id" doc:"Environment ID"`
	FileName string `path:"fileName" doc:"mTLS asset filename"`
}

// NewHandler builds the environment HTTP handler and its stream producer.
func NewHandler(
	environmentService *EnvironmentService,
	settingsService *settings.SettingsService,
	apiKeyService *apikey.ApiKeyService,
	eventService *event.EventService,
	cfg *config.Config,
	activityService activity.Service,
) *EnvironmentHandler {
	return &EnvironmentHandler{
		environmentService: environmentService,
		settingsService:    settingsService,
		apiKeyService:      apiKeyService,
		eventService:       eventService,
		cfg:                cfg,
		streamHub:          agg.NewHub[[]environment.Environment](),
		activityService:    activityService,
	}
}

// ListEnvironments returns a paginated list of environments.
func (h *EnvironmentHandler) ListEnvironments(ctx context.Context, input *ListEnvironmentsInput) (*handlerutil.Page[environment.Environment], error) {
	// The list endpoint backs both the environments management page and the
	// environment switcher, so any authenticated caller may reach it. Global
	// listers (sudo, global admins, or holders of the org-level
	// environments:list permission) see every environment; environment-scoped
	// callers see only the environments they hold at least one permission on.
	ps, ok := middleware.PermissionsFromContext(ctx)
	if !ok {
		return nil, huma.Error403Forbidden("permission denied")
	}
	var accessibleEnvIDs []string // nil = no restriction
	if !environmentListerSeesAllInternal(ps) {
		accessibleEnvIDs = accessibleEnvironmentIDsInternal(ps)
	}

	params := handlerutil.PaginationParams(input.Start, input.Limit, input.Sort, input.Order, input.Search)
	if input.Type != "" {
		params.Filters["type"] = input.Type
	}

	envs, paginationResp, err := h.environmentService.ListEnvironmentsPaginated(ctx, params, accessibleEnvIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("Failed to fetch environments")
	}
	for i := range envs {
		h.applyEdgeRuntimeStateInternal(ctx, &envs[i])
	}

	return &handlerutil.Page[environment.Environment]{
		Body: base.Paginated[environment.Environment]{
			Success:    true,
			Data:       envs,
			Pagination: handlerutil.PaginationResponse(paginationResp),
		},
	}, nil
}

// environmentListerSeesAllInternal reports whether the caller may list every
// environment. True for sudo callers, global admins, and holders of the
// org-level environments:list permission (Allows short-circuits on sudo and
// treats global admins as holding every permission).
func environmentListerSeesAllInternal(ps *authz.PermissionSet) bool {
	return ps != nil && ps.Allows(authz.PermEnvironmentsList, "")
}

// accessibleEnvironmentIDsInternal returns the sorted set of environment IDs the
// caller holds at least one environment-scoped permission on. A non-nil result
// (possibly empty) restricts the environment list for non-global callers; an
// empty result yields no environments.
func accessibleEnvironmentIDsInternal(ps *authz.PermissionSet) []string {
	if ps == nil {
		return []string{}
	}
	ids := slices.Sorted(maps.Keys(ps.PerEnv))
	return ids
}

// visibleEnvironmentsForInternal filters the shared environment snapshot down
// to what the caller may see. It reuses the same access rules as
// ListEnvironments so the stream and the REST list can never disagree about
// which environments a caller has. The input slice is shared across stream
// subscribers and must not be mutated.
func visibleEnvironmentsForInternal(envs []environment.Environment, ps *authz.PermissionSet) []environment.Environment {
	if environmentListerSeesAllInternal(ps) {
		return envs
	}

	allowed := make(map[string]struct{}, len(ps.PerEnv))
	for _, envID := range accessibleEnvironmentIDsInternal(ps) {
		allowed[envID] = struct{}{}
	}

	filtered := make([]environment.Environment, 0, len(allowed))
	for _, env := range envs {
		if _, ok := allowed[env.ID]; ok {
			filtered = append(filtered, env)
		}
	}
	return filtered
}

func (h *EnvironmentHandler) RunStreamProducer(ctx context.Context, ps *authz.PermissionSet, events chan<- environment.StreamEvent) {
	var lastFingerprint uint64
	var haveFingerprint bool
	var lastSentAt time.Time

	h.streamHub.Subscribe(ctx, environmentStreamHubKeyInternal, h.runEnvironmentStreamListerInternal, func(all []environment.Environment) bool {
		envs := visibleEnvironmentsForInternal(all, ps)
		fingerprint := kit.Fingerprint(envs)
		// Re-send unchanged state on a floor so relative timestamps in the UI
		// ("last seen 2 minutes ago") keep advancing.
		if haveFingerprint && fingerprint == lastFingerprint && time.Since(lastSentAt) < environmentStreamRefreshFloor {
			return true
		}
		lastFingerprint = fingerprint
		haveFingerprint = true
		lastSentAt = time.Now()

		return agg.Send(ctx, events, environment.StreamEvent{
			Type:         "snapshot",
			Environments: envs,
			Timestamp:    time.Now(),
		})
	})
}

func (h *EnvironmentHandler) runEnvironmentStreamListerInternal(ctx context.Context, publish func([]environment.Environment)) {
	changes, unsubscribe := h.environmentService.SubscribeRuntimeChanges()
	defer unsubscribe()

	var rows []environment.Environment
	loaded := false
	publishRuntime := func() {
		if !loaded {
			return
		}
		envs := slices.Clone(rows)
		for i := range envs {
			h.environmentService.ApplyHostRuntimeState(ctx, &envs[i])
		}
		publish(envs)
	}
	reload := func() {
		envs, err := h.environmentService.ListVisibleEnvironments(ctx)
		if err != nil {
			// A failed read must not end the stream; the next wake-up retries.
			if ctx.Err() == nil {
				slog.WarnContext(ctx, "environment stream failed to list environments", "error", err)
			}
			return
		}
		rows = envs
		loaded = true
		publishRuntime()
	}

	reload()

	runtimeTicker := time.NewTicker(environmentStreamRuntimeInterval)
	defer runtimeTicker.Stop()
	reloadTicker := time.NewTicker(environmentStreamReloadInterval)
	defer reloadTicker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-changes:
			reload()
		case <-reloadTicker.C:
			reload()
		case <-runtimeTicker.C:
			if !loaded {
				reload()
				continue
			}
			publishRuntime()
		}
	}
}
