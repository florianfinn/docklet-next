// Source: getarcaneapp/arcane/backend/internal/role/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed OIDC and passkeys.; Service-account filter removed with the federated account cut.

package role

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/samber/mo"
)

func validatePermissionsInternal(perms []string) error {
	for _, p := range perms {
		if !authz.IsKnownPermission(p) {
			return common.Classify(common.ErrUnknownPermission, errors.New("Unknown permission: "+

				p))
		}
	}
	return nil
}

// ValidatePermissionsAgainstCaller rejects any permission in `desired` that the
// caller does not hold at global scope. Sudo callers (agent / env access
// tokens, bootstrap paths) bypass entirely. Holding a permission only inside a
// specific environment is intentionally insufficient: roles are reusable
// templates that can later be assigned globally, so an env-scoped grant must
// not let the caller mint a global-capable role.
//
// Unknown permission strings are rejected first with an UnknownPermissionError
// so a caller typo-ing a permission gets a descriptive 400 instead of a
// misleading 403 from the escalation guard below (which would always fire on
// an unknown perm because no PermissionSet contains it). This also gives the
// escalation loop a clean invariant: every perm reaching it is real.
//
// Callers should run this before persisting role permissions to defend against
// privilege escalation if the role mutation endpoints are ever exposed beyond
// global admins.
func (s *RoleService) ValidatePermissionsAgainstCaller(caller *authz.PermissionSet, desired []string) error {
	if err := validatePermissionsInternal(desired); err != nil {
		return err
	}
	return validatePermissionSetAgainstCallerInternal(caller, desired, "")
}

// ValidateRoleAssignmentAgainstCaller rejects assigning a role at the requested
// scope when the caller does not hold every permission in that role at that
// same scope.
func (s *RoleService) ValidateRoleAssignmentAgainstCaller(ctx context.Context, caller *authz.PermissionSet, roleID string, environmentID *string) error {
	role, err := s.GetRole(ctx, roleID)
	if err != nil {
		return err
	}

	desired := []string(role.Permissions)
	if validatePermissionsErr := validatePermissionsInternal(desired); validatePermissionsErr != nil {
		return validatePermissionsErr
	}
	return validatePermissionSetAgainstCallerInternal(caller, desired, mo.PointerToOption(environmentID).OrEmpty())
}

func validatePermissionSetAgainstCallerInternal(caller *authz.PermissionSet, desired []string, environmentID string) error {
	if caller == nil {
		if len(desired) == 0 {
			return nil
		}
		return common.Classify(common.ErrRolePermissionEscalation, errors.New("cannot grant a permission you do not hold: "+

			desired[0]))
	}
	if caller.Sudo {
		return nil
	}
	for _, p := range desired {
		if !caller.Allows(p, environmentID) {
			return common.Classify(common.ErrRolePermissionEscalation, errors.New("cannot grant a permission you do not hold: "+

				p))
		}
	}
	return nil
}
