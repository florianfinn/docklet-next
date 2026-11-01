// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/access"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
	"strings"
)

var userCapabilityOperations = map[string]string{
	"listStacks": "list-projects", "listHosts": "listEnvironments", "getStack": "get-project",
	"runStackCommand": "deploy-project", "runContainerCommand": "start-container", "restartService": "restart-project",
	"subscribeLogs":          "GET /api/environments/:id/ws/projects/:projectId/logs",
	"subscribeContainerLogs": "GET /api/environments/:id/ws/containers/:containerId/logs",
	"subscribeMetrics":       "GET /api/environments/:id/ws/containers/:containerId/stats",
	"openContainerShell":     "GET /api/environments/:id/ws/containers/:containerId/terminal",
	"saveStackFiles":         "update-project-workspace", "setStackLabels": "update-project-tag", "getProjectWorkspace": "get-project-workspace",
	"readProjectFile": "get-project-workspace-file", "downloadProjectFile": "download-project-workspace-file", "updateProjectWorkspace": "update-project-workspace",
	"checkUpdates": "check-project-updates", "installStackUpdates": "update-project-images", "getUpdateBaseline": "get-update-baseline",
}

func userCapabilities(ctx context.Context, in *GetCapabilitiesInput, ps *authz.PermissionSet) (*handlerutil.Out[[]capability.Descriptor], error) {
	p := access.FromPermissions(ps)
	env, raw, _ := strings.Cut(in.ID, "~")
	params := access.Params{ProjectID: raw, ContainerID: raw}
	if in.Kind == "stack" || in.Kind == "container" {
		op := "get-project"
		if in.Kind == "container" {
			op = "get-container"
		}
		if d := access.Decide(ctx, p, op, env, params); d != access.Allow {
			return nil, access.Problem(d.Status(), string(d))
		}
	}
	out := capabilityDescriptorsInternal(capability.ResourceRef{Kind: in.Kind, ID: in.ID})
	for i := range out {
		op := userCapabilityOperations[out[i].OperationID]
		if in.Kind == "stack" && out[i].OperationID == "subscribeMetrics" {
			op = "get-project-runtime"
		}
		d := access.Decide(ctx, p, op, env, params)
		out[i].Available = d == access.Allow
		out[i].DisabledReasonCode = string(d)
	}
	return &handlerutil.Out[[]capability.Descriptor]{Body: base.ApiResponse[[]capability.Descriptor]{Success: true, Data: out}}, nil
}
