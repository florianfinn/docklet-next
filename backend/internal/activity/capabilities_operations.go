// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D6 permission-first operation descriptors with fresh readers; D7 host maintenance and restart/settings refusal parity. M14 host maintenance and availability.

package activity

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	"github.com/florianfinn/docklet-next/types/capability"
	dto "github.com/florianfinn/docklet-next/types/container"
	environmentDTO "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

// Readers use current host state and the same inspect/override guard as admission.
// They are invoked only after the resource and grants have been validated.
type OperationCapabilityReaders struct {
	HostReason          func(context.Context, string) string
	HostOperationReason func(context.Context, string) string
	RestartReason       func(context.Context, string) string
	SettingsWriteReason func(context.Context) string
	GuardOrigin         func(context.Context, string, string) error
	Origin              func(context.Context, string, string) (dto.OriginFields, error)
}
type operationCapabilityKey struct{}

func WithOperationCapabilityReaders(ctx context.Context, readers OperationCapabilityReaders) context.Context {
	return context.WithValue(ctx, operationCapabilityKey{}, readers)
}
func OperationCapabilityContext(readers OperationCapabilityReaders) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Operation().OperationID == "get-capabilities" {
			ctx = huma.WithContext(ctx, WithOperationCapabilityReaders(ctx.Context(), readers))
		}
		next(ctx)
	}
}

func operationCapability(ctx context.Context, operation string, ref capability.ResourceRef, ps *authz.PermissionSet) string {
	grant := authz.PermSettingsRead
	if operation == "saveSettings" {
		grant = authz.PermSettingsWrite
	}
	if operation == "getSettings" || operation == "saveSettings" {
		if ref.Kind == "hub" && ref.ID == "0" && ps.Allows(grant, "0") {
			if operation == "saveSettings" {
				readers, ok := ctx.Value(operationCapabilityKey{}).(OperationCapabilityReaders)
				if !ok || readers.SettingsWriteReason == nil {
					return "capability.unavailable"
				}
				return readers.SettingsWriteReason(ctx)
			}
			return ""
		}
		return "capability.unavailable"
	}
	if operation == "updateHost" || operation == "connectHost" || operation == "reconnectHost" || operation == "restartAgent" {
		if ref.Kind != "host" || ref.ID == "" || strings.Contains(ref.ID, "~") || (operation == "restartAgent" && ref.ID == "0") {
			return "capability.unavailable"
		}
		allowed := ps.IsGlobalAdmin()
		if operation == "connectHost" || operation == "reconnectHost" {
			allowed = ps.Allows(authz.PermEnvironmentsRead, ref.ID)
		}
		if !allowed {
			return "capability.unavailable"
		}
		readers, ok := ctx.Value(operationCapabilityKey{}).(OperationCapabilityReaders)
		if !ok || readers.HostOperationReason == nil || readers.RestartReason == nil {
			return "capability.unavailable"
		}
		ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
		defer cancel()
		if reason := readers.RestartReason(ctx, ref.ID); reason != "" {
			return reason
		}
		if operation == "restartAgent" {
			if readers.HostReason == nil {
				return "capability.unavailable"
			}
			return readers.HostReason(ctx, ref.ID)
		}
		return readers.HostOperationReason(ctx, ref.ID)
	}
	env, raw, scoped := strings.Cut(ref.ID, "~")
	kind, grants := "container", []string{authz.PermContainersEdit}
	if operation == "recreateContainer" {
		grants = append(grants, authz.PermActivitiesRead)
	}
	if operation == "restartService" {
		kind, grants = "stack", deployRuntimeGrantsInternal
	}
	if ref.Kind != kind || !scoped || env == "" || raw == "" || strings.Contains(raw, "~") {
		return "capability.unavailable"
	}
	for _, required := range grants {
		if !ps.Allows(required, env) {
			return "capability.unavailable"
		}
	}
	readers, ok := ctx.Value(operationCapabilityKey{}).(OperationCapabilityReaders)
	if !ok || readers.HostReason == nil || readers.RestartReason == nil {
		return "capability.unavailable"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if reason := readers.RestartReason(ctx, env); reason != "" {
		return reason
	}
	if reason := readers.HostReason(ctx, env); reason != "" {
		return reason
	}
	if operation == "restartService" {
		return ""
	}
	if operation == "recreateContainer" {
		if readers.GuardOrigin == nil {
			return "capability.unavailable"
		}
		return operationCapabilityError(readers.GuardOrigin(ctx, env, raw))
	}
	if readers.Origin == nil {
		return "capability.unavailable"
	}
	fields, err := readers.Origin(ctx, env, raw)
	if err != nil {
		return operationCapabilityError(err)
	}
	// The operation covers assignment and reset. An unconfirmed override stays
	// resettable; the write still assigns only while detection says unknown.
	if fields.Origin != dto.OriginUnknown && fields.OriginSource != dto.OriginUnconfirmed {
		return "container.origin-locked"
	}
	return ""
}

func operationCapabilityError(err error) string {
	if err == nil {
		return ""
	}
	var problem *dto.Problem
	if errors.As(err, &problem) {
		return problem.Code
	}
	var status huma.StatusError
	if errors.As(err, &status) && status.GetStatus() == 404 {
		return "capability.unavailable"
	}
	_, code := remenv.AgentAccessFailure(err)
	return code
}

// RestartCapabilityReason shares the admission predicate and fails closed on read errors.
func RestartCapabilityReason(ctx context.Context, db *gorm.DB, env string) string {
	if db == nil {
		return "capability.unavailable"
	}
	if err := runtimeCheckRestartTx(db.WithContext(ctx), env); err != nil {
		var problem *environmentDTO.HostProblem
		if errors.As(err, &problem) && problem.Code == "host.restarting" {
			return "host.restarting"
		}
		return "capability.unavailable"
	}
	return ""
}
