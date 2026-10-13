// Source: getarcaneapp/arcane/backend/internal/port/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package port

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

func RegisterPorts(api huma.API, portSvc *PortService) {
	h := &PortHandler{portService: portSvc}

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "list-ports",
		Method:      http.MethodGet,
		Path:        "/environments/{id}/ports",
		Summary:     "List port mappings",
		Tags:        []string{"Ports"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermContainersList, h.ListPorts)
}
