// Source: getarcaneapp/arcane/backend/internal/version/module.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

package version

import (
	"github.com/danielgtaylor/huma/v2"
)

// RegisterVersion registers version endpoints.
func RegisterVersion(api huma.API, versionService *VersionService) {
	h := &VersionHandler{versionService: versionService}

	huma.Register(api, huma.Operation{
		OperationID: "getVersion",
		Method:      "GET",
		Path:        "/version",
		Summary:     "Get version information",
		Description: "Get application version information and check for updates",
		Tags:        []string{"Version"},
		Security:    []map[string][]string{},
	}, h.GetVersion)

	huma.Register(api, huma.Operation{
		OperationID: "getAppVersion",
		Method:      "GET",
		Path:        "/app-version",
		Summary:     "Get app version",
		Description: "Get the current application version",
		Tags:        []string{"Version"},
		Security:    []map[string][]string{},
	}, h.GetAppVersion)
}
