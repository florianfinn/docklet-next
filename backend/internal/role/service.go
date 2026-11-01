// Source: getarcaneapp/arcane/backend/internal/role/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.; Service-account filter removed with the federated account cut; M15 simple-user rights and identity contracts.

package role

import (
	"context"
	"encoding/json/v2"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
	"maps"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	roletypes "github.com/florianfinn/docklet-next/types/role"
	"github.com/samber/hot"
	"go.getarcane.app/kit/normalization"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
)

const (
	permissionCacheTTL = 60 * time.Second
)

type RoleService struct {
	UserChanged    func(string)
	db             *database.DB
	userCache      *hot.HotCache[string, *authz.PermissionSet]
	apiKeyCache    *hot.HotCache[string, *authz.PermissionSet]
	cacheFillMu    sync.Mutex
	userCacheGen   atomic.Uint64
	apiKeyCacheGen atomic.Uint64
}

func NewRoleService(db *database.DB) *RoleService {
	return &RoleService{
		db: db,
		userCache: hot.NewHotCache[string, *authz.PermissionSet](hot.LRU, 2048).
			WithTTL(permissionCacheTTL).
			WithJanitor().
			Build(),
		apiKeyCache: hot.NewHotCache[string, *authz.PermissionSet](hot.LRU, 2048).
			WithTTL(permissionCacheTTL).
			WithJanitor().
			Build(),
	}
}

func (s *RoleService) EnsureBuiltInRoles(ctx context.Context) error {
	builtIns := map[string]struct {
		name string
		desc string
		perm []string
	}{
		authz.BuiltInRoleAdmin:         {"Admin", "Full administrative access", authz.AllPermissions()},
		authz.BuiltInRoleEditor:        {"Editor", "Read and write on Docker resources", authz.BuiltInEditorPermissions()},
		authz.BuiltInRoleNoShellEditor: {"No-Shell Editor", "Editor without interactive container shell access", authz.BuiltInNoShellEditorPermissions()},
		authz.BuiltInRoleDeployer:      {"Deployer", "Deploy and lifecycle containers and projects", authz.BuiltInDeployerPermissions()},
		authz.BuiltInRoleMonitor:       {"Monitor", "Observability-only access: logs, dashboards, events", authz.BuiltInMonitorPermissions()},
		authz.BuiltInRoleViewer:        {"Viewer", "Read-only access to all resources", authz.BuiltInViewerPermissions()},
	}

	return dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		for id, spec := range builtIns {
			role := Role{
				ID:          id,
				Name:        spec.name,
				Description: new(spec.desc),
				Permissions: database.StringSlice(spec.perm),
				BuiltIn:     true,
			}
			if err := tx.Save(&role).Error; err != nil {
				return fmt.Errorf("failed to upsert built-in role %s: %w", id, err)
			}
		}
		return nil
	})
}

// AssertGlobalAdminExists returns common.ErrNoGlobalAdminRemains if zero
// non-service users resolve to global administrator permissions. Called at boot
// after role reconciliation; also called from inside mutation paths.
func (s *RoleService) AssertGlobalAdminExists(ctx context.Context) error {
	count, err := s.CountGlobalAdminsExcludingUserInDB(ctx, s.db.WithContext(ctx), "")
	if err != nil {
		return err
	}
	if count == 0 {
		return common.Classify(common.ErrNoGlobalAdminRemains, errors.New("At least one user must retain a global Admin role assignment")) //nolint:staticcheck // Preserve the existing error message.
	}
	return nil
}

func (s *RoleService) ListRoles(ctx context.Context, params pagination.QueryParams) ([]Role, pagination.Response, error) {
	var roles []Role
	query := s.db.WithContext(ctx).Model(&Role{})

	if term := strings.TrimSpace(params.Search); term != "" {
		pattern := "%" + term + "%"
		query = query.Where("name LIKE ? OR COALESCE(description, '') LIKE ?", pattern, pattern)
	}

	resp, err := pagination.PaginateAndSortDB(params, query, &roles)
	if err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to paginate roles: %w", err)
	}
	return roles, resp, nil
}

func (s *RoleService) ListAllRoles(ctx context.Context) ([]Role, error) {
	var roles []Role
	if err := s.db.WithContext(ctx).Order("name").Find(&roles).Error; err != nil {
		return nil, fmt.Errorf("failed to list roles: %w", err)
	}
	return roles, nil
}

func (s *RoleService) GetRole(ctx context.Context, id string) (*Role, error) {
	return dbutil.FirstWhere[Role](ctx, s.db.DB, common.Classify(common.ErrRoleNotFound, errors.New("Role not found")), "id = ?", id)
}

func (s *RoleService) CreateRole(ctx context.Context, name string, description *string, permissions []string) (*Role, error) {
	input := roletypes.CreateRole{Name: name, Description: description, Permissions: permissions}
	if err := normalization.Normalize(&input); err != nil {
		return nil, err
	}
	if err := validatePermissionsInternal(permissions); err != nil {
		return nil, err
	}
	role := &Role{
		Name:        input.Name,
		Description: input.Description,
		Permissions: database.StringSlice(permissions),
		BuiltIn:     false,
	}
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var conflict int64
		if err := tx.Model(&Role{}).Where("name = ?", input.Name).Count(&conflict).Error; err != nil {
			return fmt.Errorf("failed to check role name uniqueness: %w", err)
		}
		if conflict > 0 {
			return common.Classify(common.ErrRoleNameTaken, errors.New("Role name already in use"))
		}
		if err := tx.Create(role).Error; err != nil {
			if dbutil.IsUniqueConstraint(err) {
				return common.ErrRoleNameTaken
			}
			return err
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return role, nil
}

// assignedUserIDsInternal validates affected users inside the role transaction.
// Their permission caches are invalidated only after commit.
func assignedUserIDsInternal(tx *gorm.DB, roleID string) ([]string, error) {
	var ids []string
	if err := tx.Model(&UserRoleAssignment{}).
		Where("role_id = ?", roleID).
		Distinct("user_id").
		Pluck("user_id", &ids).Error; err != nil {
		return nil, fmt.Errorf("failed to list users assigned to role: %w", err)
	}
	if len(ids) == 0 {
		return ids, nil
	}
	var present []string
	if err := tx.Table("users").
		Where("id IN ?", ids).
		Order("id").
		Pluck("id", &present).Error; err != nil {
		return nil, fmt.Errorf("failed to read users assigned to role: %w", err)
	}
	if len(present) != len(ids) {
		return nil, common.ErrUserNotFound
	}
	return ids, nil
}

func (s *RoleService) UpdateRole(ctx context.Context, id, name string, description *string, permissions []string) (*Role, error) {
	input := roletypes.UpdateRole{Name: name, Description: description, Permissions: permissions}
	if err := normalization.Normalize(&input); err != nil {
		return nil, err
	}
	if err := validatePermissionsInternal(permissions); err != nil {
		return nil, err
	}
	var out Role
	var affected []string
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var existing Role
		if err := tx.Where("id = ?", id).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.Classify(common.ErrRoleNotFound, errors.New("Role not found"))
			}
			return fmt.Errorf("failed to load role: %w", err)
		}
		if existing.BuiltIn {
			return common.Classify(common.ErrRoleBuiltIn, errors.New("Built-in role cannot be modified")) //nolint:staticcheck // Preserve the existing error message.
		}
		if input.Name != existing.Name {
			var conflict int64
			if err := tx.Model(&Role{}).Where("name = ? AND id <> ?", input.Name, id).Count(&conflict).Error; err != nil {
				return fmt.Errorf("failed to check role name uniqueness: %w", err)
			}
			if conflict > 0 {
				return common.Classify(common.ErrRoleNameTaken, errors.New("Role name already in use"))
			}
		}
		ids, err := assignedUserIDsInternal(tx, id)
		if err != nil {
			return err
		}
		affected = ids
		existing.Name = input.Name
		existing.Description = input.Description
		existing.Permissions = permissions
		result := tx.Model(&Role{}).Where("id = ?", id).Select("*").Updates(&existing)
		if updateRoleErr := result.Error; updateRoleErr != nil {
			if dbutil.IsUniqueConstraint(updateRoleErr) {
				return common.ErrRoleNameTaken
			}
			return fmt.Errorf("failed to update role: %w", updateRoleErr)
		}
		if result.RowsAffected != 1 {
			return common.ErrRoleNotFound
		}
		out = existing
		return nil
	})
	if err != nil {
		return nil, err
	}
	for _, userID := range affected {
		s.InvalidateUser(userID)
	}
	return &out, nil
}

func (s *RoleService) DeleteRole(ctx context.Context, id string) error {
	var affected []string
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var existing Role
		if err := tx.Where("id = ?", id).First(&existing).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.Classify(common.ErrRoleNotFound, errors.New("Role not found"))
			}
			return fmt.Errorf("failed to load role: %w", err)
		}
		if existing.BuiltIn {
			return common.Classify(common.ErrRoleBuiltIn, errors.New("Built-in role cannot be modified")) //nolint:staticcheck // Preserve the existing error message.
		}

		// Collect affected users before deleting the role.
		ids, err := assignedUserIDsInternal(tx, id)
		if err != nil {
			return err
		}
		affected = ids
		result := tx.Delete(&Role{}, "id = ?", id)
		if deleteRoleErr := result.Error; deleteRoleErr != nil {
			return fmt.Errorf("failed to delete role: %w", deleteRoleErr)
		}
		if result.RowsAffected != 1 {
			return common.ErrRoleNotFound
		}
		return nil
	})
	if err != nil {
		return err
	}
	// Invalidate caches AFTER the transaction commits so a concurrent
	// cache-miss cannot re-populate with stale data from the not-yet-visible
	// delete. Consistent with UpdateRole / SetUserAssignments.
	for _, uid := range affected {
		s.InvalidateUser(uid)
	}
	return nil
}

// CountUsersAssignedToRole returns how many distinct users hold an assignment
// to the given role (any source, any environment scope).
func (s *RoleService) CountUsersAssignedToRole(ctx context.Context, roleID string) (int, error) {
	var count int64
	if err := s.db.WithContext(ctx).
		Model(&UserRoleAssignment{}).
		Distinct("user_id").
		Where("role_id = ?", roleID).
		Count(&count).Error; err != nil {
		return 0, fmt.Errorf("failed to count users assigned to role: %w", err)
	}
	return int(count), nil
}

// ---------- User role assignments ----------

func (s *RoleService) ListUserAssignments(ctx context.Context, userID string) ([]UserRoleAssignment, error) {
	var out []UserRoleAssignment
	if err := s.db.WithContext(ctx).
		Where("user_id = ?", userID).
		Order("source ASC, role_id ASC").
		Find(&out).Error; err != nil {
		return nil, fmt.Errorf("failed to list user assignments: %w", err)
	}
	return out, nil
}

// replaceUserAssignmentsForSourceInternal replaces the user's assignments for a
// one source. References are validated inside the transaction and the
// global-admin guard is enforced before commit.

func (s *RoleService) replaceUserAssignmentsForSourceInternal(ctx context.Context, userID, source string, desired []UserRoleAssignment) error {
	for i := range desired {
		desired[i].UserID = userID
		desired[i].Source = source
	}
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		// Assignment replacement and admin validation share the immediate transaction.
		var current struct{ ID string }
		if err := tx.Table("users").
			Select("id").
			Where("id = ?", userID).
			Take(&current).Error; err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return common.ErrUserNotFound
			}
			return fmt.Errorf("failed to read user for assignment update: %w", err)
		}
		before, err := s.ResolveUserPermissionsInDB(ctx, tx, userID)
		if err != nil {
			return err
		}
		if err := validateAssignmentsExistInternal(tx, desired); err != nil {
			return err
		}
		if err := tx.Where("user_id = ? AND source = ?", userID, source).
			Delete(&UserRoleAssignment{}).Error; err != nil {
			return fmt.Errorf("failed to clear %s assignments: %w", source, err)
		}
		if len(desired) > 0 {
			if err := tx.Create(&desired).Error; err != nil {
				return fmt.Errorf("failed to insert %s assignments: %w", source, err)
			}
		}
		after, err := s.ResolveUserPermissionsInDB(ctx, tx, userID)
		if err != nil {
			return err
		}
		if actor, ok := userctx.CurrentUserFromContext(ctx); ok && actor.ID == userID && before.IsGlobalAdmin() != after.IsGlobalAdmin() {
			return errors.New("cannot change your own administrator status")
		}
		count, err := s.CountGlobalAdminsExcludingUserInDB(ctx, tx, "")
		if err != nil {
			return err
		}
		if count == 0 {
			return common.Classify(common.ErrNoGlobalAdminRemains, errors.New("At least one user must retain a global Admin role assignment")) //nolint:staticcheck // Preserve the existing error message.
		}
		return nil
	})
	if err != nil {
		return err
	}
	s.InvalidateUser(userID)
	return nil
}

// SetUserAssignments replaces the user's source='manual' assignments with the
// given desired set. Enforces the global-admin guard.
func (s *RoleService) SetUserAssignments(ctx context.Context, userID string, desired []UserRoleAssignment) error {
	return s.replaceUserAssignmentsForSourceInternal(ctx, userID, RoleAssignmentSourceManual, desired)
}

// validateAssignmentsExistInternal verifies every distinct RoleID and
// EnvironmentID referenced by `desired` exists in the database. Returns the
// first missing reference wrapped in an InvalidRoleAssignmentError so the
// handler can map it to a 400 with a descriptive message.
func validateAssignmentsExistInternal(tx *gorm.DB, desired []UserRoleAssignment) error {
	roleIDSet := make(map[string]struct{}, len(desired))
	envIDSet := make(map[string]struct{}, len(desired))
	for _, a := range desired {
		roleIDSet[a.RoleID] = struct{}{}
		if a.EnvironmentID != nil {
			envIDSet[*a.EnvironmentID] = struct{}{}
		}
	}

	if len(roleIDSet) > 0 {
		roleIDs := slices.Collect(maps.Keys(roleIDSet))
		var found []string
		if err := tx.Model(&Role{}).Where("id IN ?", roleIDs).Pluck("id", &found).Error; err != nil {
			return fmt.Errorf("failed to verify role ids: %w", err)
		}
		foundSet := make(map[string]struct{}, len(found))
		for _, id := range found {
			foundSet[id] = struct{}{}
		}
		for id := range roleIDSet {
			if _, ok := foundSet[id]; !ok {
				return common.Classify(common.ErrInvalidRoleAssignment, fmt.Errorf("invalid role assignment: role %q does not exist", id))
			}
		}
	}

	if len(envIDSet) > 0 {
		envIDs := slices.Collect(maps.Keys(envIDSet))
		var found []string
		if err := tx.Table("environments").Where("id IN ?", envIDs).Pluck("id", &found).Error; err != nil {
			return fmt.Errorf("failed to verify environment ids: %w", err)
		}
		foundSet := make(map[string]struct{}, len(found))
		for _, id := range found {
			foundSet[id] = struct{}{}
		}
		for id := range envIDSet {
			if _, ok := foundSet[id]; !ok {
				return common.Classify(common.ErrInvalidRoleAssignment, fmt.Errorf("invalid role assignment: environment %q does not exist", id))
			}
		}
	}
	return nil
}

func (s *RoleService) CountGlobalAdminsExcludingUser(ctx context.Context, excludedUserID string) (int, error) {
	return s.CountGlobalAdminsExcludingUserInDB(ctx, s.db.WithContext(ctx), excludedUserID)
}

// CountGlobalAdminsExcludingUserInDB counts effective admins on the supplied transaction.
func (s *RoleService) CountGlobalAdminsExcludingUserInDB(ctx context.Context, tx *gorm.DB, excludedUserID string) (int, error) {
	type globalPermissionRow struct {
		UserID      string `gorm:"column:user_id"`
		Permissions string `gorm:"column:permissions"`
	}
	var rows []globalPermissionRow
	query := tx.WithContext(ctx).
		Table("users AS u").
		Select("u.id AS user_id, r.permissions AS permissions").
		Joins("INNER JOIN user_role_assignments ura ON ura.user_id = u.id AND ura.environment_id IS NULL").
		Joins("INNER JOIN roles r ON r.id = ura.role_id").Where("u.disabled_at IS NULL")
	if strings.TrimSpace(excludedUserID) != "" {
		query = query.Where("u.id <> ?", excludedUserID)
	}
	if err := query.Scan(&rows).Error; err != nil {
		return 0, fmt.Errorf("failed to list global role permissions for admin count: %w", err)
	}

	permissionsByUser := make(map[string]*authz.PermissionSet, len(rows))
	for _, r := range rows {
		ps := permissionsByUser[r.UserID]
		if ps == nil {
			ps = authz.NewPermissionSet()
			permissionsByUser[r.UserID] = ps
		}
		perms, err := decodePermissionsJSONInternal(r.Permissions)
		if err != nil {
			return 0, fmt.Errorf("failed to decode role permissions: %w", err)
		}
		ps.AddGlobal(perms...)
	}

	count := 0
	for _, ps := range permissionsByUser {
		if ps.IsGlobalAdmin() {
			count++
		}
	}
	return count, nil
}

// ---------- Permission resolution ----------

// ResolvePermissions returns the effective PermissionSet for a user, caching
// the result per-user for permissionCacheTTL.
func (s *RoleService) ResolvePermissions(ctx context.Context, userID string) (*authz.PermissionSet, error) {
	if userID == "" {
		return authz.NewPermissionSet(), nil
	}
	if ps, ok, _ := s.userCache.Get(userID); ok {
		return ps, nil
	}
	gen := s.userCacheGen.Load()
	ps, err := s.ResolveUserPermissionsInDB(ctx, s.db.WithContext(ctx), userID)
	if err != nil {
		return nil, err
	}
	s.cacheFillMu.Lock()
	if s.userCacheGen.Load() == gen {
		s.userCache.Set(userID, ps)
	}
	s.cacheFillMu.Unlock()
	return ps, nil
}

// ResolveUserPermissionsInDB resolves permissions using the supplied transaction or database handle.
func (s *RoleService) ResolveUserPermissionsInDB(ctx context.Context, tx *gorm.DB, userID string) (*authz.PermissionSet, error) {
	// Scan into raw string for the permissions JSON column to avoid GORM's
	// schema-introspection on anonymous local structs (which can't see the
	// type tags needed to wire database.StringSlice's Scanner).
	type row struct {
		Permissions   string  `gorm:"column:permissions"`
		EnvironmentID *string `gorm:"column:environment_id"`
	}
	var rows []row
	if err := tx.Table("user_role_assignments AS ura").
		Select("r.permissions AS permissions, ura.environment_id AS environment_id").
		Joins("INNER JOIN roles r ON r.id = ura.role_id").
		Where("ura.user_id = ?", userID).
		Scan(&rows).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve user permissions: %w", err)
	}
	ps := authz.NewPermissionSet()
	for _, r := range rows {
		perms, err := decodePermissionsJSONInternal(r.Permissions)
		if err != nil {
			return nil, fmt.Errorf("failed to decode role permissions: %w", err)
		}
		if r.EnvironmentID == nil {
			ps.AddGlobal(perms...)
		} else {
			ps.AddEnv(*r.EnvironmentID, perms...)
		}
	}
	if ps.IsGlobalAdmin() {
		return ps, nil
	}
	rowsApp, err := access.Assignments(ctx, tx, userID)
	if err != nil {
		return nil, err
	}
	return access.Synthesize(userID, rowsApp), nil
}

// decodePermissionsJSONInternal parses the JSON-encoded `roles.permissions` column
// into a string slice. The column is `[]` for an empty role.
func decodePermissionsJSONInternal(raw string) ([]string, error) {
	if raw == "" || raw == "[]" {
		return nil, nil
	}
	var out []string
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

// ResolveApiKeyPermissions returns the PermissionSet for an API key. Caches
// per-key. Falls back to an empty set (deny-all) if the key has no perms.
func (s *RoleService) ResolveApiKeyPermissions(ctx context.Context, apiKeyID string) (*authz.PermissionSet, error) {
	if ps, ok, _ := s.apiKeyCache.Get(apiKeyID); ok {
		return ps, nil
	}
	gen := s.apiKeyCacheGen.Load()
	permissions, err := s.resolveApiKeyPermissionsInDBInternal(s.db.WithContext(ctx), apiKeyID)
	if err != nil {
		return nil, err
	}
	s.cacheFillMu.Lock()
	if s.apiKeyCacheGen.Load() == gen {
		s.apiKeyCache.Set(apiKeyID, permissions)
	}
	s.cacheFillMu.Unlock()
	return permissions, nil
}

func (s *RoleService) resolveApiKeyPermissionsInDBInternal(db *gorm.DB, apiKeyID string) (*authz.PermissionSet, error) {
	var grants []ApiKeyPermission
	if err := db.Where("api_key_id = ?", apiKeyID).Find(&grants).Error; err != nil {
		return nil, fmt.Errorf("failed to resolve api key permissions: %w", err)
	}
	permissions := authz.NewPermissionSet()
	for _, grant := range grants {
		if grant.EnvironmentID == nil {
			permissions.AddGlobal(grant.Permission)
		} else {
			permissions.AddEnv(*grant.EnvironmentID, grant.Permission)
		}
	}
	return permissions, nil
}

// SetApiKeyPermissions replaces every permission row on the given API key
// atomically. Validation that the granted permissions don't exceed the
// creator's capabilities happens in the handler layer.
func (s *RoleService) SetApiKeyPermissions(ctx context.Context, apiKeyID string, grants []ApiKeyPermission) error {
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		return s.SetApiKeyPermissionsInDB(ctx, tx, apiKeyID, grants)
	})
	if err != nil {
		return err
	}
	s.InvalidateApiKey(apiKeyID)
	return nil
}

// SetApiKeyPermissionsInDB replaces permission rows using the supplied transaction or database handle.
func (s *RoleService) SetApiKeyPermissionsInDB(ctx context.Context, tx *gorm.DB, apiKeyID string, grants []ApiKeyPermission) error {
	for i := range grants {
		grants[i].ApiKeyID = apiKeyID
	}
	if err := tx.WithContext(ctx).Where("api_key_id = ?", apiKeyID).Delete(&ApiKeyPermission{}).Error; err != nil {
		return fmt.Errorf("failed to clear api key permissions: %w", err)
	}
	if len(grants) > 0 {
		if err := tx.WithContext(ctx).Create(&grants).Error; err != nil {
			return fmt.Errorf("failed to insert api key permissions: %w", err)
		}
	}
	return nil
}

func (s *RoleService) InvalidateUser(userID string) {
	s.cacheFillMu.Lock()
	s.userCacheGen.Add(1)
	s.userCache.Delete(userID)
	s.cacheFillMu.Unlock()
	if s.UserChanged != nil {
		s.UserChanged(userID)
	}
}

// InvalidateApiKey drops the cached PermissionSet for one API key.
func (s *RoleService) InvalidateApiKey(apiKeyID string) {
	s.cacheFillMu.Lock()
	defer s.cacheFillMu.Unlock()
	s.apiKeyCacheGen.Add(1)
	s.apiKeyCache.Delete(apiKeyID)
}

// ---------- helpers ----------
