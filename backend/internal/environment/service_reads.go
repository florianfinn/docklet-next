// Source: getarcaneapp/arcane/backend/internal/environment/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed central variables and .env.global; functional cut: removed GitOps and saved Git repositories; functional cut: removed Swarm; functional cut: removed S3 backup destinations.

package environment

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/environment"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/samber/mo"
	"go.getarcane.app/kit/pkg"
	"go.getarcane.app/kit/pkg/mapping"
	"log/slog"
	"slices"
	"strings"
	"time"
)

// SyncResourcesToEnvironment tracks a manual sync and attempts every resource group.
func (s *EnvironmentService) SyncResourcesToEnvironment(ctx context.Context, environmentID string, user *usertypes.Actor, activityService activity.Service) (string, error) {
	return activity.RunHandlerActivity(ctx, activityService, activity.HandlerOptions{
		EnvironmentID:  environmentID,
		Type:           activitytypes.TypeResourceAction,
		ResourceType:   "environment",
		ResourceID:     environmentID,
		ResourceName:   environmentID,
		User:           user,
		Step:           "Syncing environment",
		Message:        "Syncing container registries",
		SuccessMessage: "Environment synced successfully",
		Metadata:       database.JSON{"action": "sync_environment"},
	}, func(ctx context.Context) error {
		var failedGroups []string

		s.ForgetSyncState(environmentID)
		if err := s.SyncRegistriesToEnvironment(ctx, environmentID); err != nil {
			slog.WarnContext(ctx, "Failed to sync registries", "environmentId", environmentID, "error", err.Error())
			failedGroups = append(failedGroups, "container registries")
		}

		if len(failedGroups) > 0 {
			return errors.New("Failed to sync " + strings.Join(failedGroups, ", ") + ". Other resource groups may have synced successfully. Check the manager logs, correct the failed sync, and retry.")
		}

		return nil
	})
}

// DisplayName returns the stored environment name or its readable fallback.
func DisplayName(environmentID, storedName string) string {
	if name := strings.TrimSpace(storedName); name != "" {
		return name
	}
	id := strings.TrimSpace(environmentID)
	return kit.Ternary(id == "" || id == LocalEnvironmentID, localEnvironmentFallbackNameInternal, id)
}

const (
	edgeTokenCacheTTL   = time.Minute
	environmentCacheTTL = 30 * time.Second
)

// GetActiveRemoteEnvironmentSnapshot returns the latest in-process snapshot for
// an enabled, visible, non-local remote environment.
func (s *EnvironmentService) GetActiveRemoteEnvironmentSnapshot(environmentID string) mo.Option[Environment] {
	if s == nil {
		return mo.None[Environment]()
	}
	return s.remoteEnvs.get(environmentID)
}

// ListActiveRemoteEnvironments returns every enabled, visible, non-local
// environment from memory, sorted by ID. The first call loads them from the
// database.
func (s *EnvironmentService) ListActiveRemoteEnvironments(ctx context.Context) ([]Environment, error) {
	s.remoteEnvs.mu.RLock()
	seeded := s.remoteEnvs.seeded
	environments := make([]Environment, 0, len(s.remoteEnvs.envs))
	for _, envRecord := range s.remoteEnvs.envs {
		environments = append(environments, envRecord)
	}
	s.remoteEnvs.mu.RUnlock()

	if !seeded {
		var err error
		if environments, err = s.ListRemoteEnvironments(ctx); err != nil {
			return nil, err
		}
	}

	slices.SortFunc(environments, func(a, b Environment) int {
		return cmp.Compare(a.ID, b.ID)
	})
	return environments, nil
}

// GetEnvironmentByIDCached is GetEnvironmentByID behind a short TTL cache that
// also remembers unknown IDs. Status and heartbeat fields may be stale.
func (s *EnvironmentService) GetEnvironmentByIDCached(ctx context.Context, id string) (*Environment, error) {
	key := environmentCacheKeyInternal{gen: s.environmentGens[environmentCacheStripeInternal(id)].Load(), id: id}
	envRecord, found, err := s.environmentCache.GetWithLoaders(key, func(keys []environmentCacheKeyInternal) (map[environmentCacheKeyInternal]Environment, error) {
		found := make(map[environmentCacheKeyInternal]Environment, len(keys))
		for _, k := range keys {
			loaded, loadErr := s.GetEnvironmentByID(ctx, k.id)
			if errors.Is(loadErr, ErrEnvironmentNotFound) {
				continue
			}
			if loadErr != nil {
				return nil, loadErr
			}
			found[k] = *loaded
		}
		return found, nil
	})
	if err != nil {
		return nil, err
	}
	if !found {
		return nil, ErrEnvironmentNotFound
	}
	return &envRecord, nil
}

// invalidateEnvironmentCacheInternal starts a new generation for id's stripe
// after a committed write, orphaning its cached record and in-flight loads.
func (s *EnvironmentService) invalidateEnvironmentCacheInternal(id string) {
	if s == nil || s.environmentCache == nil {
		return
	}
	previous := s.environmentGens[environmentCacheStripeInternal(id)].Add(1) - 1
	s.environmentCache.Delete(environmentCacheKeyInternal{gen: previous, id: id})
}

// invalidateAllEnvironmentCacheInternal covers writes whose affected IDs are unknown.
func (s *EnvironmentService) invalidateAllEnvironmentCacheInternal() {
	if s == nil || s.environmentCache == nil {
		return
	}
	for i := range s.environmentGens {
		s.environmentGens[i].Add(1)
	}
	s.environmentCache.Purge()
}

func (s *EnvironmentService) ListEnvironmentsPaginated(ctx context.Context, params pagination.QueryParams, accessibleEnvIDs []string) ([]environment.Environment, pagination.Response, error) {
	if strings.TrimSpace(params.Filters["type"]) != "" {
		return s.listEnvironmentsPaginatedWithRuntimeFiltersInternal(ctx, params, accessibleEnvIDs)
	}

	var envs []Environment
	q := s.db.WithContext(ctx).Model(&Environment{})
	// accessibleEnvIDs == nil means "no restriction". A non-nil slice limits the
	// result to those environment IDs; an empty slice therefore matches nothing.
	switch {
	case accessibleEnvIDs == nil:
		// no restriction
	case len(accessibleEnvIDs) == 0:
		q = q.Where("1 = 0")
	default:
		q = q.Where("id IN ?", accessibleEnvIDs)
	}

	if term := strings.TrimSpace(params.Search); term != "" {
		searchPattern := "%" + term + "%"
		q = q.Where(
			"name LIKE ? OR api_url LIKE ?",
			searchPattern, searchPattern,
		)
	}

	q = pagination.ApplyFilter(q, "status", params.Filters["status"])
	q = pagination.ApplyBooleanFilter(q, "enabled", params.Filters["enabled"])

	paginationResp, err := pagination.PaginateAndSortDB(params, q, &envs)
	if err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to paginate environments: %w", err)
	}

	out, mapErr := mapping.MapSlice[Environment, environment.Environment](envs)
	if mapErr != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to map environments: %w", mapErr)
	}

	return out, paginationResp, nil
}

func (
	s *EnvironmentService,
) listEnvironmentsPaginatedWithRuntimeFiltersInternal(
	ctx context.Context,
	params pagination.QueryParams,
	accessibleEnvIDs []string,
) (
	[]environment.Environment,
	pagination.Response,
	error,
) {
	var envs []Environment
	if err := s.db.WithContext(ctx).
		Model(&Environment{}).
		Find(&envs).Error; err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to list environments: %w", err)
	}

	items, mapErr := mapping.MapSlice[Environment, environment.Environment](envs)
	if mapErr != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to map environments: %w", mapErr)
	}

	// nil = no restriction; non-nil restricts to the caller's accessible envs.
	if accessibleEnvIDs != nil {
		items = filterEnvironmentsByIDInternal(items, accessibleEnvIDs)
	}

	for i := range items {
		ApplyEnvironmentRuntimeState(&items[i])
	}

	config := pagination.Config[environment.Environment]{
		SearchAccessors: []pagination.SearchAccessor[environment.Environment]{
			func(env environment.Environment) (string, error) { return env.Name, nil },
			func(env environment.Environment) (string, error) { return env.ApiUrl, nil },
		},
		SortBindings: []pagination.SortBinding[environment.Environment]{
			{
				Key: "id",
				Fn: func(a, b environment.Environment) int {
					return strings.Compare(strings.ToLower(a.ID), strings.ToLower(b.ID))
				},
			},
			{
				Key: "name",
				Fn: func(a, b environment.Environment) int {
					return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
				},
			},
			{
				Key: "status",
				Fn: func(a, b environment.Environment) int {
					return strings.Compare(strings.ToLower(a.Status), strings.ToLower(b.Status))
				},
			},
			{
				Key: "enabled",
				Fn: func(a, b environment.Environment) int {
					if a.Enabled == b.Enabled {
						return 0
					}
					return kit.Ternary(a.Enabled, 1, -1)
				},
			},
			{
				Key: "apiUrl",
				Fn: func(a, b environment.Environment) int {
					return strings.Compare(strings.ToLower(a.ApiUrl), strings.ToLower(b.ApiUrl))
				},
			},
		},
		FilterAccessors: []pagination.FilterAccessor[environment.Environment]{
			{
				Key: "status",
				Fn: func(item environment.Environment, filterValue string) bool {
					return strings.EqualFold(item.Status, strings.TrimSpace(filterValue))
				},
			},
			{
				Key: "enabled",
				Fn: func(item environment.Environment, filterValue string) bool {
					value, valid := kit.ParseBool(filterValue)
					return !valid || item.Enabled == value
				},
			},
			{
				Key: "type",
				Fn:  environmentTypeMatchesInternal,
			},
		},
	}

	result := config.SearchOrderAndPaginate(items, params)
	paginationResp := pagination.BuildResponse(result.TotalCount, result.TotalAvailable, params)

	return result.Items, paginationResp, nil
}

// ListVisibleEnvironments returns persisted rows; callers apply ApplyEnvironmentRuntimeState.
func (s *EnvironmentService) ListVisibleEnvironments(ctx context.Context) ([]environment.Environment, error) {
	var envs []Environment
	if err := s.db.WithContext(ctx).
		Model(&Environment{}).
		Order("created_at asc, id asc").
		Find(&envs).Error; err != nil {
		return nil, fmt.Errorf("failed to list visible environments: %w", err)
	}

	out, mapErr := mapping.MapSlice[Environment, environment.Environment](envs)
	if mapErr != nil {
		return nil, fmt.Errorf("failed to map environments: %w", mapErr)
	}

	return out, nil
}

// ListRemoteEnvironmentIDs returns the IDs of enabled remote environments; it
// satisfies agg.RemoteEnvironmentLister for aggregated stream handlers.
func (s *EnvironmentService) ListRemoteEnvironmentIDs(ctx context.Context) ([]string, error) {
	envs, err := s.ListRemoteEnvironments(ctx)
	if err != nil {
		return nil, err
	}
	ids := make([]string, 0, len(envs))
	for _, env := range envs {
		ids = append(ids, env.ID)
	}
	return ids, nil
}

// ListRemoteEnvironments returns all non-local, enabled environments for syncing purposes.
func (s *EnvironmentService) ListRemoteEnvironments(ctx context.Context) ([]Environment, error) {
	for {
		s.remoteEnvs.mu.RLock()
		revision := s.remoteEnvs.revision
		s.remoteEnvs.mu.RUnlock()

		var envs []Environment
		err := s.db.WithContext(ctx).
			Model(&Environment{}).
			Where("id != ?", "0").
			Where("enabled = ?", true).
			Find(&envs).Error
		if err != nil {
			return nil, fmt.Errorf("failed to list remote environments: %w", err)
		}

		s.remoteEnvs.mu.Lock()
		// Retry if a cache mutation or another refresh overtook the database read.
		if s.remoteEnvs.revision != revision {
			s.remoteEnvs.mu.Unlock()
			continue
		}
		s.remoteEnvs.envs = make(map[string]Environment, len(envs))
		for _, envRecord := range envs {
			s.remoteEnvs.envs[envRecord.ID] = envRecord
		}
		s.remoteEnvs.seeded = true
		s.remoteEnvs.revision++
		s.remoteEnvs.mu.Unlock()
		return envs, nil
	}
}

// SubscribeRuntimeChanges returns a channel that receives a coalesced wake-up
// whenever environment liveness may have changed, plus a function to release it.
