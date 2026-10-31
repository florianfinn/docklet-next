// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D6 descriptor permission, scope and fresh reason matrix. M14 host maintenance and availability.

package activity

import (
	"context"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/capability"
	dto "github.com/florianfinn/docklet-next/types/container"
	"github.com/stretchr/testify/require"
)

func TestD6OperationDescriptors(t *testing.T) {
	for _, operation := range []string{"recreateContainer", "setContainerOrigin", "restartService", "getSettings", "saveSettings"} {
		t.Run(operation, func(t *testing.T) {
			ref := capability.ResourceRef{Kind: "container", ID: "example~target"}
			grants := []string{authz.PermContainersEdit}
			if operation == "recreateContainer" {
				grants = append(grants, authz.PermActivitiesRead)
			}
			if operation == "restartService" {
				ref.Kind, grants = "stack", deployRuntimeGrantsInternal
			}
			if operation == "getSettings" {
				ref, grants = capability.ResourceRef{Kind: "hub", ID: "0"}, []string{authz.PermSettingsRead}
			}
			if operation == "saveSettings" {
				ref, grants = capability.ResourceRef{Kind: "hub", ID: "0"}, []string{authz.PermSettingsWrite}
			}
			cases := []struct {
				name, host, lock string
				deny, invalid    bool
				fields           dto.OriginFields
				want             string
			}{
				{name: "available", fields: dto.OriginFields{Origin: dto.OriginUnknown}},
				{name: "permission", deny: true, want: "capability.unavailable"},
				{name: "invalid-ref", invalid: true, want: "capability.unavailable"},
			}
			if ref.Kind != "hub" {
				for _, reason := range []string{"agent.unavailable", "agent.invalid-token", "agent.update-required"} {
					cases = append(cases, struct {
						name, host, lock string
						deny, invalid    bool
						fields           dto.OriginFields
						want             string
					}{name: reason, host: reason, want: reason})
				}
			}
			if operation == "recreateContainer" {
				cases = append(cases, struct {
					name, host, lock string
					deny, invalid    bool
					fields           dto.OriginFields
					want             string
				}{name: "origin-or-protected-self", lock: "container.origin-locked", want: "container.origin-locked"})
			}
			if operation == "setContainerOrigin" {
				for _, value := range []dto.Origin{dto.OriginCompose, dto.OriginUnraid, dto.OriginStandalone} {
					cases = append(cases, struct {
						name, host, lock string
						deny, invalid    bool
						fields           dto.OriginFields
						want             string
					}{name: string(value), fields: dto.OriginFields{Origin: value}, want: "container.origin-locked"})
				}
				cases = append(cases, struct {
					name, host, lock string
					deny, invalid    bool
					fields           dto.OriginFields
					want             string
				}{name: "unconfirmed-unknown", fields: dto.OriginFields{Origin: dto.OriginUnknown, OriginSource: dto.OriginUnconfirmed}})
				// Reset must stay reachable for an unconfirmed override on a replacement
				// detected otherwise (AUTO-5); the write refuses assignment there.
				for _, value := range []dto.Origin{dto.OriginCompose, dto.OriginUnraid, dto.OriginStandalone} {
					cases = append(cases, struct {
						name, host, lock string
						deny, invalid    bool
						fields           dto.OriginFields
						want             string
					}{name: "unconfirmed-detected-" + string(value), fields: dto.OriginFields{Origin: value, OriginSource: dto.OriginUnconfirmed}})
				}
			}
			for _, tc := range cases {
				t.Run(tc.name, func(t *testing.T) {
					env := "example"
					if ref.Kind == "hub" {
						env = "0"
					}
					ps := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{env: {}}}
					for _, grant := range grants {
						ps.PerEnv[env][grant] = struct{}{}
					}
					if tc.deny {
						delete(ps.PerEnv[env], grants[0])
						ps.PerEnv[env][authz.PermContainersRead] = struct{}{}
					}
					ctx := context.WithValue(t.Context(), middleware.ContextKeyUserPermissions, ps)
					reads := 0
					ctx = WithOperationCapabilityReaders(ctx, OperationCapabilityReaders{
						RestartReason:       func(context.Context, string) string { return "" },
						SettingsWriteReason: func(context.Context) string { return "" },
						HostReason:          func(_ context.Context, got string) string { reads++; require.Equal(t, env, got); return tc.host },
						GuardOrigin: func(_ context.Context, got, raw string) error {
							reads++
							require.Equal(t, env, got)
							require.Equal(t, "target", raw)
							if tc.lock != "" {
								return &dto.Problem{Code: tc.lock, Status: 409}
							}
							return nil
						},
						Origin: func(_ context.Context, got, raw string) (dto.OriginFields, error) {
							reads++
							require.Equal(t, env, got)
							require.Equal(t, "target", raw)
							return tc.fields, nil
						},
					})
					current := ref
					if tc.invalid {
						current.ID += "~extra"
					}
					out, err := getCapabilities(ctx, &GetCapabilitiesInput{Kind: current.Kind, ID: current.ID})
					require.NoError(t, err)
					found := false
					for _, descriptor := range out.Body.Data {
						if descriptor.OperationID == operation {
							found = true
							require.Equal(t, tc.want == "", descriptor.Available)
							require.Equal(t, tc.want, descriptor.DisabledReasonCode)
						}
					}
					require.True(t, found)
					if tc.deny || tc.invalid || ref.Kind == "hub" {
						require.Zero(t, reads)
					}
				})
			}
		})
	}
}

func TestD6ReaderIsFreshAndRequired(t *testing.T) {
	ps := authz.SudoPermissionSet()
	ref := capability.ResourceRef{Kind: "container", ID: "0~target"}
	require.Equal(t, "capability.unavailable", operationCapability(t.Context(), "recreateContainer", ref, ps))
	locked := false
	ctx := WithOperationCapabilityReaders(t.Context(), OperationCapabilityReaders{
		RestartReason:       func(context.Context, string) string { return "" },
		SettingsWriteReason: func(context.Context) string { return "" },
		HostReason:          func(context.Context, string) string { return "" },
		GuardOrigin: func(context.Context, string, string) error {
			if locked {
				return &dto.Problem{Code: "container.origin-locked"}
			}
			return nil
		},
	})
	require.Empty(t, operationCapability(ctx, "recreateContainer", ref, ps))
	locked = true
	require.Equal(t, "container.origin-locked", operationCapability(ctx, "recreateContainer", ref, ps))
}

func TestD6RecreateRequiresRuntimeReadGrant(t *testing.T) {
	ps := &authz.PermissionSet{PerEnv: map[string]map[string]struct{}{"example": {authz.PermContainersEdit: {}}}}
	reads := 0
	ctx := WithOperationCapabilityReaders(t.Context(), OperationCapabilityReaders{
		RestartReason:       func(context.Context, string) string { return "" },
		SettingsWriteReason: func(context.Context) string { return "" },
		HostReason:          func(context.Context, string) string { reads++; return "" },
		GuardOrigin:         func(context.Context, string, string) error { return nil },
	})
	require.Equal(t, "capability.unavailable", operationCapability(ctx, "recreateContainer", capability.ResourceRef{Kind: "container", ID: "example~target"}, ps))
	require.Zero(t, reads)
}
