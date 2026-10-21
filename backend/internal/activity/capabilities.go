// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package activity

import (
	"context"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
)

var adapterOperationIDs = []string{
	"checkUpdates", "completeSetup", "connectHost", "connectNetwork", "convertDockerRun", "createEntry",
	"createNetwork", "createVolume", "deleteEntry", "deleteVolume", "deployStack",
	"disconnectNetwork", "discoverHub", "getSecurity", "getSettings", "getStack",
	"getTopology", "getVolume", "importSource", "installContainerUpdate", "installStackUpdates",
	"issueAgentToken", "listActions", "listContainers", "listEvents", "listFiles",
	"listHosts", "listImages", "listSchedule", "listStacks", "listSystemComponents",
	"listVolumes", "planVolumePrune", "pruneImages", "pruneVolumes", "pullImage",
	"readFile", "reconnectHost", "recreateContainer", "registerHost", "removeHost",
	"removeImage", "removeNetwork", "renameEntry", "restartAgent", "restartService",
	"restoreBackup", "restoreRestorePoint", "runBackup", "runContainerCommand", "runScan",
	"runStackCommand", "saveSettings", "saveStackFiles", "setAutoUpdate", "setBackupPolicy",
	"setContainerAutoUpdate", "setContainerOrigin", "setScanEnabled", "setStackLabels",
	"subscribeActions", "subscribeChanges", "subscribeContainerLogs", "subscribeLogs",
	"subscribeMetrics", "testBackupTarget", "testNotification", "updateHost", "uploadFile",
	"writeFile",
}

type GetCapabilitiesInput struct {
	Kind string `query:"kind" default:"hub" doc:"Resource kind"`
	ID   string `query:"id" default:"0" doc:"Resource ID"`
}

func capabilityDescriptorsInternal(ref capability.ResourceRef) []capability.Descriptor {
	result := make([]capability.Descriptor, 0, len(adapterOperationIDs))
	for _, operationID := range adapterOperationIDs {
		descriptor := capability.Descriptor{OperationID: operationID, ResourceRef: ref}
		switch operationID {
		case "listImages", "listVolumes", "getVolume", "getTopology", "subscribeMetrics", "listStacks", "getStack", "listContainers", "listHosts", "listActions", "listEvents", "subscribeActions", "subscribeChanges":
			descriptor.Available = true
		default:
			descriptor.DisabledReasonCode = "capability.mapping-unavailable"
		}
		result = append(result, descriptor)
	}
	return result
}

func getCapabilities(ctx context.Context, input *GetCapabilitiesInput) (*handlerutil.Out[[]capability.Descriptor], error) {
	descriptors := capabilityDescriptorsInternal(capability.ResourceRef{Kind: input.Kind, ID: input.ID})
	permissions, _ := middleware.PermissionsFromContext(ctx)
	for i := range descriptors {
		switch descriptors[i].OperationID {
		case "runStackCommand", "runContainerCommand":
			ref := descriptors[i].ResourceRef
			kind := "stack"
			required := []string{authz.PermActivitiesRead, authz.PermProjectsDeploy, authz.PermProjectsDown, authz.PermProjectsRestart}
			if descriptors[i].OperationID == "runContainerCommand" {
				kind = "container"
				required = []string{authz.PermActivitiesRead, authz.PermContainersStart, authz.PermContainersStop, authz.PermContainersRestart}
			}
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := ref.Kind == kind && scoped && env != "" && raw != "" && !strings.Contains(raw, "~")
			for _, permission := range required {
				valid = valid && permissions.Allows(permission, env)
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "subscribeLogs", "subscribeContainerLogs":
			ref := descriptors[i].ResourceRef
			kind, logGrant, readGrant := "stack", authz.PermProjectsLogs, authz.PermProjectsRead
			if descriptors[i].OperationID == "subscribeContainerLogs" {
				kind, logGrant, readGrant = "container", authz.PermContainersLogs, authz.PermContainersRead
			}
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := ref.Kind == kind && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && permissions.Allows(logGrant, env) && permissions.Allows(readGrant, env)
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "deployStack", "saveStackFiles", "setStackLabels", "convertDockerRun":
			descriptors[i].Available = authoringAllowedInternal(descriptors[i].OperationID, descriptors[i].ResourceRef, permissions)
			if descriptors[i].Available {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "registerHost", "issueAgentToken", "removeHost", "discoverHub":
			descriptors[i].Available = permissions.IsGlobalAdmin()
			if descriptors[i].Available {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		}
	}
	return &handlerutil.Out[[]capability.Descriptor]{Body: base.ApiResponse[[]capability.Descriptor]{
		Success: true,
		Data:    descriptors,
	}}, nil
}

// authoringAllowedInternal scopes stack authoring to a host ref for new stacks
// and a host~project stack ref for existing ones; deploy also needs run reads.
func authoringAllowedInternal(operationID string, ref capability.ResourceRef, permissions *authz.PermissionSet) bool {
	env, raw, scoped := strings.Cut(ref.ID, "~")
	var required []string
	switch {
	case operationID == "convertDockerRun" && ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~"):
		env, required = ref.ID, []string{authz.PermContainersCreate}
	case operationID == "deployStack" && ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~"):
		env, required = ref.ID, []string{authz.PermProjectsCreate, authz.PermProjectsDeploy, authz.PermActivitiesRead}
	case ref.Kind == "stack" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && operationID != "convertDockerRun":
		required = []string{authz.PermProjectsUpdate}
		if operationID == "deployStack" {
			required = append(required, authz.PermProjectsDeploy, authz.PermActivitiesRead)
		}
	default:
		return false
	}
	for _, permission := range required {
		if !permissions.Allows(permission, env) {
			return false
		}
	}
	return true
}

func RegisterCapabilities(api huma.API) {
	huma.Register(api, huma.Operation{
		OperationID: "get-capabilities",
		Method:      http.MethodGet,
		Path:        "/capabilities",
		Summary:     "Get adapter capability descriptors",
		Tags:        []string{"Capabilities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, getCapabilities)
}
