// Source: getarcaneapp/arcane/backend/internal/network/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; typed network write problem schemas.

package network

import (
	networktypes "github.com/florianfinn/docklet-next/types/network"
	"net/http"
	"reflect"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterNetworks registers network endpoints.
func RegisterNetworks(api huma.API, networkSvc *NetworkService, dockerSvc *docker.DockerClientService, activitySvc *activity.ActivityService, appCtx handlerutil.ActivityAppContext) {
	h := &NetworkHandler{
		networkService:  networkSvc,
		dockerService:   dockerSvc,
		activityService: activitySvc,
		appCtx:          appCtx.Context(),
	}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-networks",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/networks",
		Summary:     "List networks",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksList, h.ListNetworks)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "network-counts",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/networks/counts",
		Summary:     "Network counts",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksList, h.GetNetworkCounts)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "create-network",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/networks",
		Summary:     "Create network",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksCreate, h.CreateNetwork)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-network-topology",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/networks/topology",
		Summary:     "Get network topology",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksRead, h.GetNetworkTopology)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "get-network",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/networks/{networkId}",
		Summary:     "Get network",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksRead, h.GetNetwork)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "delete-network",
		Method:      http.MethodDelete,
		Path:        "/environments/{id}/networks/{networkId}",
		Summary:     "Delete network",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksDelete, h.DeleteNetwork)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "prune-networks",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/networks/prune",
		Summary:     "Prune networks",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksPrune, h.PruneNetworks)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "connect-network-container",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/networks/{networkId}/connect",
		Summary:     "Connect container to network",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksConnect, h.ConnectContainer)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "disconnect-network-container",
		Method:      http.MethodPost,
		Path:        "/environments/{id}/networks/{networkId}/disconnect",
		Summary:     "Disconnect container from network",
		Tags:        []string{"Networks"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermNetworksDisconnect, h.DisconnectContainer)

	problem := api.OpenAPI().Components.Schemas.Schema(reflect.TypeFor[networktypes.Error](), true, "")
	for _, item := range api.OpenAPI().Paths {
		for _, op := range []*huma.Operation{item.Post, item.Delete} {
			if op == nil {
				continue
			}
			switch op.OperationID {
			case "create-network", "delete-network", "connect-network-container", "disconnect-network-container":
				for _, status := range []string{"400", "404", "409"} {
					op.Responses[status] = &huma.Response{Description: "Network write rejected", Content: map[string]*huma.MediaType{"application/problem+json": {Schema: problem}}}
				}
			}
		}
	}
}
