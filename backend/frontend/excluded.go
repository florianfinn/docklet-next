// Source: getarcaneapp/arcane/backend/frontend/excluded.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: None

//go:build exclude_frontend

package frontend

import "github.com/labstack/echo/v5"

func RegisterFrontend(e *echo.Echo) error {
	_ = e
	return ErrFrontendNotIncluded
}
