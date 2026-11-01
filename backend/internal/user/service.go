// Source: getarcaneapp/arcane/backend/internal/user/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.; Service-account filter removed with the federated account cut; M15 simple-user rights and identity contracts.

package user

import (
	"context"
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strings"

	"github.com/florianfinn/docklet-next/types/user"
	"go.getarcane.app/kit/normalization"
	"go.getarcane.app/kit/pkg"
	"golang.org/x/crypto/argon2"
	"gorm.io/gorm"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/role"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/pagination"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/dbutil"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/userctx"
)

type Argon2Params struct {
	memory      uint32
	iterations  uint32
	parallelism uint8
	saltLength  uint32
	keyLength   uint32
}

func DefaultArgon2Params() *Argon2Params {
	return &Argon2Params{
		memory:      64 * 1024,
		iterations:  3,
		parallelism: 2,
		saltLength:  16,
		keyLength:   32,
	}
}

type UserService struct {
	db                   *database.DB
	roleService          *role.RoleService
	argon2Params         *Argon2Params
	revokeSessionsExcept func(context.Context, *gorm.DB, string, string) error
}

var (
	ErrCannotRemoveLastAdmin = errors.New("cannot remove the last admin user")
	ErrUsernameRequired      = errors.New("username is required")
	ErrUsernameTaken         = errors.New("username already in use")

	// ErrInsufficientPrivilege is returned when a caller attempts to modify a
	// target whose effective privilege is equal to or higher than the caller's
	// (e.g. a delegated users:update holder trying to edit a global admin).
	ErrInsufficientPrivilege = errors.New("insufficient privilege to modify this user")
)

func NewUserService(db *database.DB, roleService *role.RoleService, revokeSessionsExcept func(context.Context, *gorm.DB, string, string) error) *UserService {
	return &UserService{
		db:                   db,
		roleService:          roleService,
		argon2Params:         DefaultArgon2Params(),
		revokeSessionsExcept: revokeSessionsExcept,
	}
}

func (s *UserService) HashPassword(password string) (string, error) {
	salt := make([]byte, s.argon2Params.saltLength)
	_, err := rand.Read(salt)
	if err != nil {
		return "", err
	}

	hash := argon2.IDKey([]byte(password), salt, s.argon2Params.iterations, s.argon2Params.memory, s.argon2Params.parallelism, s.argon2Params.keyLength)

	b64Salt := base64.RawStdEncoding.EncodeToString(salt)
	b64Hash := base64.RawStdEncoding.EncodeToString(hash)

	encodedHash := fmt.Sprintf("$argon2id$v=%d$m=%d,t=%d,p=%d$%s$%s", argon2.Version, s.argon2Params.memory, s.argon2Params.iterations, s.argon2Params.parallelism, b64Salt, b64Hash)

	return encodedHash, nil
}

func (s *UserService) ValidatePassword(encodedHash, password string) error {
	parts := strings.Split(encodedHash, "$")
	if len(parts) != 6 {
		return errors.New("invalid hash format")
	}

	var version int
	_, err := fmt.Sscanf(parts[2], "v=%d", &version)
	if err != nil {
		return err
	}
	if version != argon2.Version {
		return errors.New("incompatible version of argon2")
	}

	var memory, iterations uint32
	var parallelism uint8
	_, err = fmt.Sscanf(parts[3], "m=%d,t=%d,p=%d", &memory, &iterations, &parallelism)
	if err != nil {
		return err
	}

	salt, err := base64.RawStdEncoding.DecodeString(parts[4])
	if err != nil {
		return err
	}

	decodedHash, err := base64.RawStdEncoding.DecodeString(parts[5])
	if err != nil {
		return err
	}

	hashLen := len(decodedHash)
	if hashLen < 0 || hashLen > 0x7fffffff {
		return errors.New("invalid hash length")
	}

	comparisonHash := argon2.IDKey([]byte(password), salt, iterations, memory, parallelism, uint32(hashLen))

	// constant-time compare
	if subtle.ConstantTimeCompare(comparisonHash, decodedHash) != 1 {
		return errors.New("invalid password")
	}

	return nil
}

func (s *UserService) CreateUser(ctx context.Context, localUser *User) (*User, error) {
	if err := normalization.Normalize(localUser); err != nil {
		return nil, err
	}
	username, err := normalizeUsernameInternal(localUser.Username)
	if err != nil {
		return nil, err
	}
	localUser.Username = username

	err = dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		if createUserErr := tx.Create(localUser).Error; createUserErr != nil {
			if isDuplicateKeyErrorInternal(tx, createUserErr) {
				return ErrUsernameTaken
			}
			return fmt.Errorf("failed to create user: %w", createUserErr)
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return localUser, nil
}

func (s *UserService) GetUserByUsername(ctx context.Context, username string) (*User, error) {
	return dbutil.FirstWhere[User](ctx, s.db.DB, common.ErrUserNotFound, "username = ?", username)
}

func (s *UserService) GetUserByID(ctx context.Context, id string) (*User, error) {
	return dbutil.FirstWhere[User](ctx, s.db.DB, common.ErrUserNotFound, "id = ?", id)
}

func (s *UserService) GetUserByEmail(ctx context.Context, email string) (*User, error) {
	var users []User
	if err := s.db.DB.WithContext(ctx).Where("email = ?", email).Limit(2).Find(&users).Error; err != nil {
		return nil, fmt.Errorf("failed to query users by email: %w", err)
	}
	switch len(users) {
	case 0:
		return nil, common.ErrUserNotFound
	case 1:
		return &users[0], nil
	default:
		return nil, common.ErrAmbiguousUserEmail
	}
}

// ResolveUserPermissions returns the effective PermissionSet for the given
// user ID, or nil when no role.RoleService is wired (RBAC disabled paths).
func (s *UserService) ResolveUserPermissions(ctx context.Context, userID string) (*authz.PermissionSet, error) {
	if s.roleService == nil {
		return nil, nil
	}
	return s.roleService.ResolvePermissions(ctx, userID)
}

// checkTargetPrivilegeInternal enforces actor-vs-target privilege ordering: a
// caller may not modify a target whose effective permissions make them a global
// admin unless the caller is itself a global admin. This prevents holders of
// delegated users:update/users:delete from seizing or removing admin accounts.
// A nil actorPerms denotes a trusted internal caller (e.g. the auth service
// acting on behalf of the account owner) and skips the privilege check.
//
// Current actor/target permissions are resolved through the immediate transaction.
// Revalidation may reduce request-time authority, never elevate it.
func (s *UserService) checkTargetPrivilegeInternal(ctx context.Context, tx *gorm.DB, actorPerms *authz.PermissionSet, targetID string) error {
	revalidateActor := actorPerms != nil && !actorPerms.Sudo && actorPerms.IsGlobalAdmin() && s.roleService != nil
	actorID := ""
	if revalidateActor {
		actor, ok := userctx.CurrentUserFromContext(ctx)
		if !ok || actor == nil || actor.ID == "" {
			return ErrInsufficientPrivilege
		}
		actorID = actor.ID
	}

	userIDs := []string{targetID}
	if actorID != "" && actorID != targetID {
		userIDs = append(userIDs, actorID)
	}
	var currentUsers []User
	if err := tx.
		Where("id IN ?", userIDs).
		Order("id ASC").
		Find(&currentUsers).Error; err != nil {
		return fmt.Errorf("failed to read users: %w", err)
	}
	targetFound := false
	actorFound := actorID == ""
	for i := range currentUsers {
		targetFound = targetFound || currentUsers[i].ID == targetID
		actorFound = actorFound || currentUsers[i].ID == actorID
	}
	if !targetFound {
		return common.ErrUserNotFound
	}
	if !actorFound {
		return ErrInsufficientPrivilege
	}

	if actorPerms == nil || actorPerms.Sudo || s.roleService == nil {
		return nil
	}
	if revalidateActor {
		currentActorPerms, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, actorID)
		if err != nil {
			return fmt.Errorf("failed to resolve actor permissions: %w", err)
		}
		if currentActorPerms != nil && currentActorPerms.IsGlobalAdmin() {
			return nil
		}
	}
	targetPerms, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, targetID)
	if err != nil {
		return fmt.Errorf("failed to resolve target permissions: %w", err)
	}
	return kit.Ternary[error](targetPerms != nil && targetPerms.IsGlobalAdmin(), ErrInsufficientPrivilege, nil)
}

// UpdateUser persists the given user. actorPerms identifies the caller
// performing the change; authenticated global-admin callers must also be
// present in ctx. Pass nil for internal service-to-service calls.
func (s *UserService) UpdateUser(ctx context.Context, localUser *User, actorPerms *authz.PermissionSet) (*User, error) {
	if err := normalization.Normalize(localUser); err != nil {
		return nil, err
	}
	passwordReset := false
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		if err := s.checkTargetPrivilegeInternal(ctx, tx, actorPerms, localUser.ID); err != nil {
			return err
		}
		var current User
		if err := tx.First(&current, "id = ?", localUser.ID).Error; err != nil {
			return err
		}
		passwordReset = localUser.RequiresPasswordChange && localUser.PasswordHash != current.PasswordHash
		result := tx.Model(&User{}).Where("id = ?", localUser.ID).Select("*").Omit("disabled_at").Updates(localUser)
		if err := result.Error; err != nil {
			if isDuplicateKeyErrorInternal(tx, err) {
				return ErrUsernameTaken
			}
			return fmt.Errorf("failed to update user: %w", err)
		}
		if result.RowsAffected != 1 {
			return common.ErrUserNotFound
		}
		if passwordReset && s.revokeSessionsExcept != nil {
			return s.revokeSessionsExcept(ctx, tx, localUser.ID, "")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	if passwordReset {
		s.RightsChanged(localUser.ID)
	} else if s.roleService != nil {
		if ps, e := s.roleService.ResolvePermissions(ctx, localUser.ID); e == nil && !ps.IsGlobalAdmin() {
			s.RightsChanged(localUser.ID)
		}
	}
	return localUser, nil
}

func normalizeUsernameInternal(username string) (string, error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return "", ErrUsernameRequired
	}
	return username, nil
}

func isDuplicateKeyErrorInternal(db *gorm.DB, err error) bool {
	if errors.Is(err, gorm.ErrDuplicatedKey) {
		return true
	}
	translator, ok := db.Dialector.(gorm.ErrorTranslator)
	return ok && errors.Is(translator.Translate(err), gorm.ErrDuplicatedKey)
}

// SetPassword hashes and persists a new password for the given user and clears
// the first-login password-change requirement for trusted internal callers.
func (s *UserService) SetPassword(ctx context.Context, localUser *User, password string) (*User, error) {
	if localUser == nil {
		return nil, errors.New("user is nil")
	}

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	localUser.PasswordHash = hashedPassword
	localUser.RequiresPasswordChange = false
	return s.UpdateUser(ctx, localUser, nil)
}

// SetPasswordAndRevokeSessionsExcept atomically sets a new password, clears
// the first-login password-change requirement, and revokes every session for
// the user except exceptSessionID. Pass an empty exceptSessionID to revoke all
// sessions. This is for trusted internal callers.
func (s *UserService) SetPasswordAndRevokeSessionsExcept(ctx context.Context, localUser *User, password, exceptSessionID string) (*User, error) {
	if localUser == nil {
		return nil, errors.New("user is nil")
	}
	if strings.TrimSpace(localUser.ID) == "" {
		return nil, common.ErrUserNotFound
	}

	hashedPassword, err := s.HashPassword(password)
	if err != nil {
		return nil, fmt.Errorf("failed to hash password: %w", err)
	}

	var updatedUser User
	err = dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		if loadPasswordResetUserErr := tx.
			Where("id = ?", localUser.ID).
			First(&updatedUser).Error; loadPasswordResetUserErr != nil {
			if errors.Is(loadPasswordResetUserErr, gorm.ErrRecordNotFound) {
				return common.ErrUserNotFound
			}
			return fmt.Errorf("failed to load user for password reset: %w", loadPasswordResetUserErr)
		}

		updatedUser.PasswordHash = hashedPassword
		updatedUser.RequiresPasswordChange = false
		result := tx.Model(&User{}).Where("id = ?", updatedUser.ID).Updates(map[string]any{"password_hash": hashedPassword, "requires_password_change": false})
		if updatePasswordErr := result.Error; updatePasswordErr != nil {
			return fmt.Errorf("failed to update user password: %w", updatePasswordErr)
		}
		if result.RowsAffected != 1 {
			return common.ErrUserNotFound
		}
		if err := tx.First(&updatedUser, "id=?", updatedUser.ID).Error; err != nil {
			return err
		}
		return s.revokeSessionsExcept(ctx, tx, updatedUser.ID, exceptSessionID)
	})
	if err != nil {
		return nil, err
	}

	if s.roleService != nil {
		if ps, e := s.roleService.ResolvePermissions(ctx, localUser.ID); e == nil && !ps.IsGlobalAdmin() {
			s.RightsChanged(localUser.ID)
		}
	}
	*localUser = updatedUser
	return localUser, nil
}

func (s *UserService) CreateDefaultAdmin(ctx context.Context) error {
	// If users already exist and at least one effective global admin remains,
	// there is nothing to bootstrap or recover — bail before touching the
	// `arcane` username at all. The username is mutable, so treating it as a
	// provenance marker would let anyone with users:create/users:update rename
	// an account to `arcane` and get promoted on the next restart.
	if s.roleService != nil {
		var count int64
		if err := s.db.WithContext(ctx).Model(&User{}).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to count users: %w", err)
		}
		if count > 0 {
			admins, err := s.roleService.CountGlobalAdminsExcludingUser(ctx, "")
			if err != nil {
				return fmt.Errorf("failed to count global admins: %w", err)
			}
			if admins > 0 {
				return nil
			}
		}
	}

	hashedPassword, err := s.HashPassword("arcane-admin")
	if err != nil {
		return fmt.Errorf("failed to hash default admin password: %w", err)
	}

	var adminUserID string
	err = dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		var count int64
		if err := tx.Model(&User{}).Count(&count).Error; err != nil {
			return fmt.Errorf("failed to count users: %w", err)
		}

		if count == 0 {
			email := "admin@localhost"
			displayName := "Arcane Admin"
			userModel := &User{
				Username:               "arcane",
				Email:                  new(email),
				DisplayName:            new(displayName),
				PasswordHash:           hashedPassword,
				RequiresPasswordChange: true,
			}
			if createAdminUserErr := tx.Create(userModel).Error; createAdminUserErr != nil {
				return fmt.Errorf("failed to create default admin user: %w", createAdminUserErr)
			}
			adminUserID = userModel.ID
			slog.InfoContext(ctx, "👑 Default admin user created!")
			slog.InfoContext(ctx, "🔑 Username: arcane")
			slog.InfoContext(ctx, "🔑 Password: arcane-admin")
			slog.InfoContext(ctx, "⚠️  User will be prompted to change password on first login")
			return nil
		}

		// Users exist but no global admin does (recovery only — the early
		// return above skips this path on healthy installs). See if `arcane`
		// is present; if not, leave the install alone.
		var existing User
		err := tx.Where("username = ?", "arcane").First(&existing).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				return nil
			}
			return fmt.Errorf("failed to look up default admin user: %w", err)
		}
		adminUserID = existing.ID
		return nil
	})
	if err != nil {
		return err
	}

	if adminUserID == "" || s.roleService == nil {
		return nil
	}
	return s.grantDefaultAdminRoleInternal(ctx, adminUserID)
}

// grantDefaultAdminRoleInternal grants the global Admin role to the bootstrap
// user on fresh bootstrap or zero-admin recovery. Preserve manual assignments;
// other sources remain managed by their own assignment writers.
func (s *UserService) grantDefaultAdminRoleInternal(ctx context.Context, adminUserID string) error {
	assignments, err := s.roleService.ListUserAssignments(ctx, adminUserID)
	if err != nil {
		return fmt.Errorf("failed to list default admin assignments: %w", err)
	}
	for _, a := range assignments {
		if a.RoleID == authz.BuiltInRoleAdmin && a.EnvironmentID == nil {
			return nil // already has it
		}
	}
	manual := make([]role.UserRoleAssignment, 0, len(assignments)+1)
	for _, a := range assignments {
		if a.Source == role.RoleAssignmentSourceManual {
			manual = append(manual, role.UserRoleAssignment{RoleID: a.RoleID, EnvironmentID: a.EnvironmentID})
		}
	}
	manual = append(manual, role.UserRoleAssignment{
		RoleID:        authz.BuiltInRoleAdmin,
		EnvironmentID: nil,
	})
	if setUserAssignmentsErr := s.roleService.SetUserAssignments(ctx, adminUserID, manual); setUserAssignmentsErr != nil {
		return fmt.Errorf("failed to grant default admin global role: %w", setUserAssignmentsErr)
	}
	slog.InfoContext(ctx, "Default admin granted global Admin role assignment", "userId", adminUserID)
	return nil
}

// DeleteUser removes the user with the given id. actorPerms identifies the
// caller performing the deletion; authenticated global-admin callers must also
// be present in ctx. Pass nil for internal service-to-service calls. A
// non-admin caller may not delete a global admin target.
func (s *UserService) DeleteUser(ctx context.Context, id string, actorPerms *authz.PermissionSet) error {
	err := dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		if s.roleService != nil {
			ps, err := s.roleService.ResolveUserPermissionsInDB(ctx, tx, id)
			if err != nil {
				return err
			}
			if ps != nil && ps.IsGlobalAdmin() {
				remaining, err := s.roleService.CountGlobalAdminsExcludingUserInDB(ctx, tx, id)
				if err != nil {
					return err
				}
				if remaining == 0 {
					return ErrCannotRemoveLastAdmin
				}
			}
		}
		if err := s.checkTargetPrivilegeInternal(ctx, tx, actorPerms, id); err != nil {
			return err
		}
		if err := tx.Delete(&User{}, "id = ?", id).Error; err != nil {
			return fmt.Errorf("failed to delete user: %w", err)
		}
		return nil
	})
	if err == nil {
		s.RightsChanged(id)
	}
	return err
}

func (s *UserService) ListUsersPaginated(ctx context.Context, params pagination.QueryParams) ([]user.User, pagination.Response, error) {
	var users []User
	query := s.db.WithContext(ctx).Model(&User{})

	if term := strings.TrimSpace(params.Search); term != "" {
		searchPattern := "%" + term + "%"
		query = query.Where(
			"username LIKE ? OR COALESCE(email, '') LIKE ? OR COALESCE(display_name, '') LIKE ?",
			searchPattern, searchPattern, searchPattern,
		)
	}

	paginationResp, err := pagination.PaginateAndSortDB(params, query, &users)
	if err != nil {
		return nil, pagination.Response{}, fmt.Errorf("failed to paginate users: %w", err)
	}

	result := s.toUserResponseDtosInternal(ctx, users)

	return result, paginationResp, nil
}

func (s *UserService) ToUserResponseDto(ctx context.Context, u User) (user.User, error) {
	dto := s.toUserResponseDtoInternal(ctx, u)
	if dto.IsGlobalAdmin && s.roleService != nil {
		remaining, err := s.roleService.CountGlobalAdminsExcludingUser(ctx, u.ID)
		if err != nil {
			return user.User{}, fmt.Errorf("failed to check whether user can be deleted: %w", err)
		}
		dto.CanDelete = remaining > 0
	}
	return dto, nil
}

// toUserResponseDtosInternal maps a user list. CanDelete is false on the only
// effective global admin.
func (s *UserService) toUserResponseDtosInternal(ctx context.Context, users []User) []user.User {
	result := make([]user.User, len(users))
	for i, u := range users {
		result[i] = s.toUserResponseDtoInternal(ctx, u)
	}
	if s.roleService == nil {
		return result
	}
	if total, err := s.roleService.CountGlobalAdminsExcludingUser(ctx, ""); err == nil && total == 1 {
		for i := range result {
			if result[i].IsGlobalAdmin {
				result[i].CanDelete = false
			}
		}
	}
	return result
}

// toUserResponseDtoInternal builds the public User DTO. RoleAssignments and
// PermissionsByEnv come from the RBAC service. Callers apply the last-admin guard.
func (s *UserService) toUserResponseDtoInternal(ctx context.Context, u User) user.User {
	dto := user.User{
		ID:                     u.ID,
		Username:               u.Username,
		DisplayName:            u.DisplayName,
		Email:                  u.Email,
		CanDelete:              true,
		Locale:                 u.Locale,
		TimeFormat:             u.TimeFormat,
		FontSize:               u.FontSize,
		Preferences:            u.Preferences,
		RequiresPasswordChange: u.RequiresPasswordChange,
		DisabledAt:             u.DisabledAt,
		Assignments:            []user.AppAssignment{},
		CreatedAt:              u.CreatedAt.Format("2006-01-02T15:04:05.999999Z"),
		UpdatedAt:              u.UpdatedAt.Format("2006-01-02T15:04:05.999999Z"),
		RoleAssignments:        []user.RoleAssignmentSummary{},
		PermissionsByEnv:       map[string][]string{},
	}
	if u.LastLogin != nil {
		lastLogin := u.LastLogin.Format("2006-01-02T15:04:05.999999Z")
		dto.LastLogin = &lastLogin
	}
	// Populate AvatarURL when the user has a custom avatar stored
	if u.HasAvatar {
		avatarURL := fmt.Sprintf("/api/users/%s/avatar", u.ID)
		dto.AvatarURL = &avatarURL
	}
	if s.roleService == nil {
		return dto
	}
	if rows, err := s.roleService.ListUserAssignments(ctx, u.ID); err == nil {
		dto.RoleAssignments = make([]user.RoleAssignmentSummary, len(rows))
		for i, r := range rows {
			dto.RoleAssignments[i] = user.RoleAssignmentSummary{
				RoleID:        r.RoleID,
				EnvironmentID: r.EnvironmentID,
				Source:        r.Source,
			}
		}
	}
	if ps, err := s.roleService.ResolvePermissions(ctx, u.ID); err == nil && ps != nil {
		dto.IsGlobalAdmin = ps.IsGlobalAdmin()
		dto.PermissionsByEnv = permissionSetToMap(ps)
	}
	dto.IsAdmin = dto.IsGlobalAdmin
	if rows, err := s.AppAssignments(ctx, u.ID); err == nil {
		dto.Assignments = rows
	}
	return dto
}

// permissionSetToMap flattens a PermissionSet into the wire format consumed
// by the frontend: a map from environment ID (or "global") to a list of
// permission strings. Sudo callers expose "*" under "global" as a sentinel
// meaning "every permission".
func permissionSetToMap(ps *authz.PermissionSet) map[string][]string {
	out := map[string][]string{}
	if ps == nil {
		return out
	}
	if ps.Sudo {
		out["global"] = []string{"*"}
		return out
	}
	if len(ps.Global) > 0 {
		globals := slices.Collect(maps.Keys(ps.Global))
		out["global"] = globals
	}
	for envID, perms := range ps.PerEnv {
		list := slices.Collect(maps.Keys(perms))
		out[envID] = list
	}
	return out
}

func (s *UserService) GetUser(ctx context.Context, userID string) (*User, error) {
	slog.DebugContext(ctx, "GetUser called", "userId", userID)
	return s.getUserInternal(ctx, userID, s.db.DB)
}

func (s *UserService) getUserInternal(ctx context.Context, userID string, tx *gorm.DB) (*User, error) {
	var userRecord User
	err := tx.WithContext(ctx).
		Where("id = ?", userID).
		First(&userRecord).
		Error
	return &userRecord, err
}

// UploadAvatar stores avatar bytes for a user, replacing any existing avatar.
func (s *UserService) UploadAvatar(ctx context.Context, userID string, data []byte, mimeType string) error {
	return dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		avatar := UserAvatar{
			UserID:   userID,
			Data:     data,
			MimeType: mimeType,
		}
		if err := tx.Save(&avatar).Error; err != nil {
			return fmt.Errorf("failed to save avatar data: %w", err)
		}

		err := tx.Model(&User{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"has_avatar": true,
			}).Error
		if err != nil {
			return fmt.Errorf("failed to update user avatar flag: %w", err)
		}
		return nil
	})
}

// DeleteAvatar removes the avatar for a user.
func (s *UserService) DeleteAvatar(ctx context.Context, userID string) error {
	return dbutil.WithTx(ctx, s.db.DB, func(tx *gorm.DB) error {
		if err := tx.Where("user_id = ?", userID).Delete(&UserAvatar{}).Error; err != nil {
			return fmt.Errorf("failed to delete avatar data: %w", err)
		}

		err := tx.Model(&User{}).
			Where("id = ?", userID).
			Updates(map[string]any{
				"has_avatar": false,
			}).Error
		if err != nil {
			return fmt.Errorf("failed to update user avatar flag: %w", err)
		}
		return nil
	})
}

// GetAvatar returns the raw avatar data and MIME type for a user.
// Returns (nil, "", nil) when the user has no custom avatar.
func (s *UserService) GetAvatar(ctx context.Context, userID string) ([]byte, string, error) {
	u, err := s.GetUserByID(ctx, userID)
	if err != nil {
		return nil, "", err
	}
	if !u.HasAvatar {
		return nil, "", nil
	}
	var avatar UserAvatar
	if loadAvatarErr := s.db.DB.WithContext(ctx).Where("user_id = ?", userID).First(&avatar).Error; loadAvatarErr != nil {
		return nil, "", kit.Ternary(errors.Is(loadAvatarErr, gorm.ErrRecordNotFound), nil, loadAvatarErr)
	}
	return avatar.Data, avatar.MimeType, nil
}
