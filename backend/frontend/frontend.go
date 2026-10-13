// Source: getarcaneapp/arcane/backend/frontend/frontend.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: ; functional cut: removed Svelte-specific metadata or asset paths.

//go:build !exclude_frontend

package frontend

import (
	"cmp"
	"embed"
	"fmt"
	"io/fs"
	"mime"
	"path"
	"strings"

	"github.com/labstack/echo/v5"
	"github.com/labstack/echo/v5/middleware"
)

//go:embed all:dist
var frontendFS embed.FS

const indexHtmlFileConstant = "index.html"

// RegisterFrontend serves embedded assets and the SPA fallback outside /api.
func RegisterFrontend(e *echo.Echo) error {
	distFS, err := fs.Sub(frontendFS, "dist")
	if err != nil {
		return fmt.Errorf("failed to create sub FS: %w", err)
	}

	if err := mime.AddExtensionType(".webmanifest", "application/manifest+json"); err != nil {
		return fmt.Errorf("failed to register web manifest MIME type: %w", err)
	}
	assetsFS, err := fs.Sub(distFS, "assets")
	if err != nil {
		return fmt.Errorf("failed to create assets sub FS: %w", err)
	}

	skipAPI := func(c *echo.Context) bool {
		return strings.HasPrefix(c.Request().URL.Path, "/api")
	}

	e.Use(middleware.GzipWithConfig(middleware.GzipConfig{
		MinLength: 1024,
		Skipper: func(c *echo.Context) bool {
			if skipAPI(c) {
				return true
			}
			switch strings.ToLower(path.Ext(c.Request().URL.Path)) {
			case ".woff2", ".woff", ".png", ".jpg", ".jpeg", ".webp", ".avif", ".ico", ".gz":
				return true
			}
			return false
		},
	}))

	e.Use(func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			if skipAPI(c) {
				return next(c)
			}

			p := cmp.Or(strings.TrimPrefix(c.Request().URL.Path, "/"), indexHtmlFileConstant)
			if _, statErr := fs.Stat(distFS, p); statErr != nil && !strings.HasPrefix(p, "assets/") {
				p = indexHtmlFileConstant
			}

			header := c.Response().Header()
			switch {
			case p == indexHtmlFileConstant || p == "service-worker.js" || p == "app.webmanifest" || p == "version.json":
				header.Set("Cache-Control", "no-cache, no-store, must-revalidate")
				header.Set("Pragma", "no-cache")
				header.Set("Expires", "0")
			case strings.HasPrefix(p, "assets/"):
				header.Set("Cache-Control", "public, max-age=31536000, immutable")
			default:
				header.Set("Cache-Control", "public, max-age=86400")
			}
			return next(c)
		}
	})

	// Missing assets return 404 so a stale page does not receive HTML as JavaScript.
	e.StaticFS("/assets/", assetsFS)
	e.Use(middleware.StaticWithConfig(middleware.StaticConfig{
		Root:       ".",
		Filesystem: distFS,
		HTML5:      true,
		Skipper: func(c *echo.Context) bool {
			return skipAPI(c) || strings.HasPrefix(c.Request().URL.Path, "/assets/")
		},
	}))

	return nil
}
