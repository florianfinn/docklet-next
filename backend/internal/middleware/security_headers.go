// Source: docklet-next/issues/670
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original shared hub and agent response hardening.

package middleware

import "github.com/labstack/echo/v5"

// SecurityHeaders protects UI and API responses while preserving a handler's stricter CSP.
func SecurityHeaders() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			response := echo.NewResponse(c.Response(), c.Logger())
			c.SetResponse(response)
			response.Before(func() {
				h := c.Response().Header()
				h.Set("X-Content-Type-Options", "nosniff")
				h.Set("X-Frame-Options", "DENY")
				if h.Get("Content-Security-Policy") == "" {
					h.Set("Content-Security-Policy", "frame-ancestors 'none'")
				}
				h.Set("Referrer-Policy", "same-origin")
			})
			return next(c)
		}
	}
}
