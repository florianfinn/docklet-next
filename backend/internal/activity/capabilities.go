// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38; scoped network write capabilities for #203; image and registry capabilities for #229; backup capabilities for #284; notification and Auto Heal settings capabilities for #297; D6 origin, recreate, service restart and hub settings descriptors. M14 host maintenance and availability.
package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/containershell"
	"net/http"
	"slices"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	"github.com/florianfinn/docklet-next/types/capability"
)

var adapterOperationIDs = []string{
	"listContainerRegistries", "getContainerRegistry", "createContainerRegistry", "updateContainerRegistry",
	"clearContainerRegistryCredential", "testContainerRegistry", "deleteContainerRegistry",
	"getUpdateBaseline", "listRestorePoints", "deleteRestorePoint", "listHostRestorePoints", "deleteHostRestorePoint", "getAutoUpdatePolicy", "setAutoUpdateApply", "saveUpdateSchedule",
	"checkUpdates", "completeSetup", "connectHost", "connectNetwork", "convertDockerRun", "createEntry",
	"createNetwork", "createVolume", "deleteEntry", "deleteVolume", "deployStack",
	"getScanSettings", "saveScanSettings", "cancelScan",
	"disconnectNetwork", "discoverHub", "getSecurity", "getSettings", "getStack",
	"openContainerShell", "getTopology", "getVolume", "importSource", "installContainerUpdate", "installStackUpdates",
	"issueAgentToken", "listActions", "listContainers", "listEvents", "listFiles",
	"listHosts", "listImages", "listSchedule", "listStacks", "listSystemComponents",
	"listVolumes", "planVolumePrune", "pruneImages", "pruneVolumes", "pullImage",
	"readFile", "reconnectHost", "recreateContainer", "registerHost", "removeHost",
	"removeImage", "removeNetwork", "renameEntry", "renameVolume", "restartAgent", "restartService",
	"restoreRestorePoint", "runContainerCommand", "runScan",
	"runStackCommand", "saveSettings", "saveStackFiles", "setAutoUpdate",
	"setContainerAutoUpdate", "setContainerOrigin", "setScanEnabled", "setStackLabels",
	"subscribeActions", "subscribeChanges", "subscribeContainerLogs", "subscribeLogs",
	"subscribeMetrics", "updateHost", "uploadFile",
	"writeFile", "downloadFile", "getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace",
}

type GetCapabilitiesInput struct {
	Kind string `query:"kind" default:"hub" doc:"Resource kind"`
	ID   string `query:"id" default:"0" doc:"Resource ID"`
}

func capabilityDescriptorsInternal(ref capability.ResourceRef) []capability.Descriptor {
	result := make([]capability.Descriptor, 0, len(adapterOperationIDs)+len(backupOperationIDs)+len(settingsOperationIDs))
	for _, operationID := range slices.Concat(adapterOperationIDs, backupOperationIDs, settingsOperationIDs) {
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
		case "getSecurity", "runScan", "getScanSettings", "saveScanSettings", "setScanEnabled", "cancelScan":
			ref := descriptors[i].ResourceRef
			grant := authz.PermVulnsRead
			if descriptors[i].OperationID == "runScan" {
				grant = authz.PermVulnsScan
			}
			if descriptors[i].OperationID == "cancelScan" {
				grant = authz.PermActivitiesCancel
			}
			valid := ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~") && permissions.Allows(grant, ref.ID)
			if descriptors[i].OperationID == "runScan" && ref.Kind == "image" {
				env, raw, scoped := strings.Cut(ref.ID, "~")
				valid = scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && permissions.Allows(grant, env)
			}
			if ref.Kind == "hub" && ref.ID == "0" {
				valid = permissions.AllowsAny(grant)
			}
			if descriptors[i].OperationID == "getScanSettings" || descriptors[i].OperationID == "saveScanSettings" || descriptors[i].OperationID == "setScanEnabled" {
				grant = authz.PermSettingsWrite
				if descriptors[i].OperationID == "getScanSettings" {
					grant = authz.PermSettingsRead
				}
				valid = ref.Kind == "hub" && ref.ID == "0" && permissions.Allows(grant, "0")
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "checkUpdates", "installStackUpdates", "restoreRestorePoint", "getUpdateBaseline", "listRestorePoints", "deleteRestorePoint", "listHostRestorePoints", "deleteHostRestorePoint", "getAutoUpdatePolicy", "setAutoUpdate", "setAutoUpdateApply", "listSchedule", "saveUpdateSchedule", "listSystemComponents":
			descriptors[i].Available = updateCapabilityAllowed(descriptors[i].OperationID, descriptors[i].ResourceRef, permissions)
			if descriptors[i].Available {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "runStackCommand", "runContainerCommand":
			ref := descriptors[i].ResourceRef
			kind := "stack"
			required := deployRuntimeGrantsInternal
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
		case "recreateContainer", "setContainerOrigin", "restartService", "getSettings", "saveSettings", "updateHost", "connectHost", "reconnectHost", "restartAgent":
			reason := operationCapability(ctx, descriptors[i].OperationID, descriptors[i].ResourceRef, permissions)
			descriptors[i].Available = reason == ""
			descriptors[i].DisabledReasonCode = reason
		case "openContainerShell":
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := ref.Kind == "container" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && permissions.Allows(authz.PermContainersExec, env)
			reason := "capability.unavailable"
			if valid {
				reason = containershell.Availability(ctx, env, raw)
			}
			descriptors[i].Available = valid && reason == ""
			descriptors[i].DisabledReasonCode = reason
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
		case "listImages", "pullImage", "removeImage", "pruneImages":
			grant := map[string]string{"listImages": authz.PermImagesList, "pullImage": authz.PermImagesPull, "removeImage": authz.PermImagesDelete, "pruneImages": authz.PermImagesPrune}[descriptors[i].OperationID]
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := descriptors[i].OperationID != "removeImage" && ref.Kind == "host" && ref.ID != "" && !scoped && permissions.Allows(grant, ref.ID)
			if descriptors[i].OperationID == "removeImage" && ref.Kind == "image" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") {
				valid = permissions.Allows(grant, env)
			}
			if ref.Kind == "hub" && ref.ID == "0" {
				valid = permissions.AllowsAny(grant)
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "listContainerRegistries", "getContainerRegistry", "createContainerRegistry", "updateContainerRegistry", "clearContainerRegistryCredential", "testContainerRegistry", "deleteContainerRegistry":
			grant := map[string]string{"listContainerRegistries": authz.PermRegistriesList, "getContainerRegistry": authz.PermRegistriesRead, "createContainerRegistry": authz.PermRegistriesCreate, "updateContainerRegistry": authz.PermRegistriesUpdate, "clearContainerRegistryCredential": authz.PermRegistriesUpdate, "testContainerRegistry": authz.PermRegistriesTest, "deleteContainerRegistry": authz.PermRegistriesDelete}[descriptors[i].OperationID]
			ref := descriptors[i].ResourceRef
			valid := ref.Kind == "hub" && ref.ID == "0"
			if descriptors[i].OperationID != "listContainerRegistries" && descriptors[i].OperationID != "createContainerRegistry" && ref.Kind == "registry" && ref.ID != "" && !strings.Contains(ref.ID, "~") {
				valid = true
			}
			// Registry configuration is hub-owned; environment grants never apply.
			valid = valid && permissions.Allows(grant, "")
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "createNetwork", "removeNetwork", "connectNetwork", "disconnectNetwork":
			grant := map[string]string{"createNetwork": authz.PermNetworksCreate, "removeNetwork": authz.PermNetworksDelete, "connectNetwork": authz.PermNetworksConnect, "disconnectNetwork": authz.PermNetworksDisconnect}[descriptors[i].OperationID]
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := descriptors[i].OperationID == "createNetwork" && ref.Kind == "host" && ref.ID != "" && !scoped && permissions.Allows(grant, ref.ID)
			if descriptors[i].OperationID != "createNetwork" && ref.Kind == "network" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") {
				valid = permissions.Allows(grant, env)
			}
			if ref.Kind == "hub" {
				valid = permissions.AllowsAny(grant)
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "listVolumes", "getVolume", "createVolume", "deleteVolume", "renameVolume", "planVolumePrune", "pruneVolumes":
			grant := map[string]string{"listVolumes": authz.PermVolumesList, "getVolume": authz.PermVolumesRead, "createVolume": authz.PermVolumesCreate, "deleteVolume": authz.PermVolumesDelete, "renameVolume": authz.PermVolumesRename, "planVolumePrune": authz.PermVolumesPrune, "pruneVolumes": authz.PermVolumesPrune}[descriptors[i].OperationID]
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := descriptors[i].OperationID != "deleteVolume" && descriptors[i].OperationID != "renameVolume" && ref.Kind == "host" && ref.ID != "" && !scoped && permissions.Allows(grant, ref.ID)
			if (descriptors[i].OperationID == "deleteVolume" || descriptors[i].OperationID == "renameVolume") && ref.Kind == "volume" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") {
				valid = permissions.Allows(grant, env)
			}
			if ref.Kind == "hub" {
				valid = permissions.AllowsAny(grant)
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "getProjectWorkspace", "readProjectFile", "downloadProjectFile", "updateProjectWorkspace":
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			grant := authz.PermProjectsRead
			if descriptors[i].OperationID == "updateProjectWorkspace" {
				grant = authz.PermProjectsUpdate
			}
			valid := ref.Kind == "stack" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && permissions.Allows(grant, env)
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "listFiles", "readFile", "downloadFile", "writeFile", "createEntry", "uploadFile", "renameEntry", "deleteEntry":
			ref := descriptors[i].ResourceRef
			env, raw, scoped := strings.Cut(ref.ID, "~")
			valid := ref.Kind == "volume" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && !strings.HasPrefix(raw, "bind:") && permissions.Allows(authz.PermVolumesRead, env)
			switch descriptors[i].OperationID {
			case "writeFile", "createEntry", "uploadFile":
				valid = valid && permissions.Allows(authz.PermVolumesUpload, env)
			case "renameEntry":
				valid = valid && permissions.Allows(authz.PermVolumesUpload, env) && permissions.Allows(authz.PermVolumesDelete, env)
			case "deleteEntry":
				valid = valid && permissions.Allows(authz.PermVolumesDelete, env)
			}
			descriptors[i].Available = valid
			if valid {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "setBackupPolicy", "runBackup", "restoreBackup", "restoreBackupFiles", "browseBackup", "downloadBackup", "deleteBackup", "uploadRestore", "getBackupOverview", "listBackupOverviews", "getBackupRepository", "checkBackupRepository", "listBackupTargets", "createBackupTarget", "updateBackupTarget", "deleteBackupTarget", "testBackupTarget", "getBackupTargetReferences", "getBackupRecoveryKey", "attachLocalBackupRecoveryKey":
			descriptors[i].Available = backupCapabilityAllowed(descriptors[i].OperationID, descriptors[i].ResourceRef, permissions)
			if descriptors[i].Available {
				descriptors[i].DisabledReasonCode = ""
			} else {
				descriptors[i].DisabledReasonCode = "capability.unavailable"
			}
		case "listNotificationTargets", "saveNotificationTarget", "deleteNotificationTarget", "testNotificationTarget", "getAutoHealSettings", "saveAutoHealSettings":
			descriptors[i].Available = settingsCapabilityAllowedInternal(descriptors[i].OperationID, descriptors[i].ResourceRef, permissions)
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

// deployRuntimeGrantsInternal matches the runStackCommand descriptor, because a
// deploy is admitted through that guarded command after the writes.
var deployRuntimeGrantsInternal = []string{authz.PermActivitiesRead, authz.PermProjectsDeploy, authz.PermProjectsDown, authz.PermProjectsRestart}

// authoringAllowedInternal scopes stack authoring to a host ref for new stacks
// and a host~project stack ref for existing ones. The hub ref only gates the
// entry point and is available when some environment allows a new deploy.
func authoringAllowedInternal(operationID string, ref capability.ResourceRef, permissions *authz.PermissionSet) bool {
	if operationID == "deployStack" && ref.Kind == "hub" {
		// One environment must hold every grant; global grants apply to each.
		required := append([]string{authz.PermProjectsCreate}, deployRuntimeGrantsInternal...)
		candidates := []string{"*"}
		if permissions != nil {
			for envID := range permissions.PerEnv {
				candidates = append(candidates, envID)
			}
		}
		for _, envID := range candidates {
			usable := true
			for _, permission := range required {
				usable = usable && permissions.Allows(permission, envID)
			}
			if usable {
				return true
			}
		}
		return false
	}
	env, raw, scoped := strings.Cut(ref.ID, "~")
	var required []string
	switch {
	case operationID == "convertDockerRun" && ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~"):
		env, required = ref.ID, []string{authz.PermContainersCreate}
	case operationID == "deployStack" && ref.Kind == "host" && ref.ID != "" && !strings.Contains(ref.ID, "~"):
		env, required = ref.ID, append([]string{authz.PermProjectsCreate}, deployRuntimeGrantsInternal...)
	case ref.Kind == "stack" && scoped && env != "" && raw != "" && !strings.Contains(raw, "~") && operationID != "convertDockerRun":
		required = []string{authz.PermProjectsUpdate}
		if operationID == "deployStack" {
			required = append(required, deployRuntimeGrantsInternal...)
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

func RegisterCapabilities(api huma.API, readers ...middleware.PointAccessReader) {
	var reader middleware.PointAccessReader
	if len(readers) > 0 {
		reader = readers[0]
	}
	huma.Register(api, huma.Operation{
		OperationID: "get-capabilities",
		Method:      http.MethodGet,
		Path:        "/capabilities",
		Summary:     "Get adapter capability descriptors",
		Tags:        []string{"Capabilities"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, func(ctx context.Context, in *GetCapabilitiesInput) (*handlerutil.Out[[]capability.Descriptor], error) {
		return getCapabilitiesWithPoints(ctx, in, reader)
	})
}

func getCapabilitiesWithPoints(ctx context.Context, in *GetCapabilitiesInput, reader middleware.PointAccessReader) (*handlerutil.Out[[]capability.Descriptor], error) {
	out, err := getCapabilities(ctx, in)
	if err != nil {
		return out, err
	}
	ps, _ := middleware.PermissionsFromContext(ctx)
	checked, allowed := false, false
	for i := range out.Body.Data {
		d := &out.Body.Data[i]
		if (d.OperationID != "listRestorePoints" && d.OperationID != "deleteRestorePoint") || !d.Available || ps.IsGlobalAdmin() {
			continue
		}
		if !checked {
			env, target, _ := strings.Cut(in.ID, "~")
			allowed = reader != nil && reader(ctx, env, target, "") == nil
			checked = true
		}
		if !allowed {
			d.Available = false
			d.DisabledReasonCode = "capability.unavailable"
		}
	}
	return out, nil
}
