// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package middleware

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"net/http"
)

func ClearCSRFCookie(ctx context.Context) string {
	secure := cookie.SecureCookieFromContext(ctx)
	name := "docklet_csrf"
	if secure {
		name = "__Host-docklet_csrf"
	}
	return (&http.Cookie{Name: name, Value: "", Path: "/", MaxAge: -1, Secure: secure, HttpOnly: true, SameSite: http.SameSiteStrictMode}).String()
}
