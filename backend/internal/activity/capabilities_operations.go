// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D6 permission-first operation descriptors with fresh readers.

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
)

// Readers use current host state and the same inspect/override guard as admission.
// They are invoked only after the resource and grants have been validated.
type OperationCapabilityReaders struct {
	HostReason  func(context.Context, string) string
	GuardOrigin func(context.Context, string, string) error
	Origin      func(context.Context, string, string) (dto.OriginFields, error)
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
			return ""
		}
		return "capability.unavailable"
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
	if !ok || readers.HostReason == nil {
		return "capability.unavailable"
	}
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
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
