// Source: getarcaneapp/arcane/backend/internal/registry/children/browse/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace

package browse

import (
	"github.com/danielgtaylor/huma/v2"

	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
)

// RegisterRoutes registers the registry browse endpoints.
func RegisterRoutes(api huma.API, h *Handler) {
	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "listContainerRegistryRepositories",
		Method:      "GET",
		Path:        "/container-registries/{id}/repositories",
		Summary:     "List registry repositories",
		Description: "List the repositories stored in a container registry through its catalog API",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermRegistriesBrowse, h.ListRepositories)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "listContainerRegistryTags",
		Method:      "GET",
		Path:        "/container-registries/{id}/tags",
		Summary:     "List repository tags",
		Description: "List the tags of a registry repository with their manifest details",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermRegistriesBrowse, h.ListTags)

	middleware.RegisterWithPermission(api, huma.Operation{
		OperationID: "deleteContainerRegistryTag",
		Method:      "DELETE",
		Path:        "/container-registries/{id}/tags",
		Summary:     "Delete a repository tag",
		Description: "Delete the manifest a tag points to, which also removes every tag sharing its digest",
		Tags:        []string{"Container Registries"},
		Security:    handlerutil.DefaultOperationSecurity(),
	}, authz.PermRegistriesDeleteTags, h.DeleteTag)
}
