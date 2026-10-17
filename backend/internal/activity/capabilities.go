// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package activity

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
)

var adapterOperationIDs = []string{
	"checkUpdates", "completeSetup", "connectHost", "connectNetwork", "createEntry",
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
		case "listStacks", "getStack", "listContainers", "listHosts", "listActions", "listEvents", "subscribeActions", "subscribeChanges":
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
