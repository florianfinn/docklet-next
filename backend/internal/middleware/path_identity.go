// Source: docklet-next/issues/264
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original single route identity guard for environment paths.

package middleware

import (
	"net/http"
	"strings"
	"unicode"
	"unicode/utf8"

	requestDTO "github.com/florianfinn/docklet-next/types/request"
	"github.com/labstack/echo/v5"
)

const environmentPathPrefix = "/api/environments/"

// IsEnvironmentPath reports whether a decoded request path belongs to the
// environment namespace that only the guarded Echo router may serve.
func IsEnvironmentPath(decoded string) bool {
	return strings.HasPrefix(decoded, environmentPathPrefix)
}

// PathIdentityGuard runs before every consumer of an environment path so the
// router, hub checks and both agent rebuilds see the same segments (#264):
// an encoded unreserved byte, an encoded `/ ? # % \` or decoded whitespace or
// control characters or malformed UTF-8 answer 400 request.path-encoded; an empty, `.` or `..`
// segment (including a trailing `/`) answers like an unmatched route.
func PathIdentityGuard(notFound echo.HandlerFunc) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c *echo.Context) error {
			decoded := c.Request().URL.Path
			if !IsEnvironmentPath(decoded) {
				return next(c)
			}
			if pathEncodingRefusedInternal(c.Request().URL.EscapedPath(), decoded) {
				return c.JSON(http.StatusBadRequest, &requestDTO.Problem{Code: requestDTO.PathEncoded, Status: http.StatusBadRequest})
			}
			for _, segment := range strings.Split(strings.TrimPrefix(decoded, environmentPathPrefix), "/") {
				if segment == "" || segment == "." || segment == ".." {
					return notFound(c)
				}
			}
			return next(c)
		}
	}
}

func pathEncodingRefusedInternal(escaped, decoded string) bool {
	for i := 0; i+2 < len(escaped); i++ {
		if escaped[i] != '%' {
			continue
		}
		b, ok := unhexByteInternal(escaped[i+1], escaped[i+2])
		if !ok {
			continue
		}
		if b < 0x80 && (isUnreservedByteInternal(b) || strings.IndexByte(`/?#%\`, b) >= 0) {
			return true
		}
	}
	if !utf8.ValidString(decoded) {
		return true
	}
	for _, r := range decoded {
		if unicode.IsSpace(r) || unicode.IsControl(r) {
			return true
		}
	}
	return false
}

func isUnreservedByteInternal(b byte) bool {
	return b >= 'A' && b <= 'Z' || b >= 'a' && b <= 'z' || b >= '0' && b <= '9' || b == '-' || b == '.' || b == '_' || b == '~'
}

func unhexByteInternal(hi, lo byte) (byte, bool) {
	h, ok1 := unhexInternal(hi)
	l, ok2 := unhexInternal(lo)
	return h<<4 | l, ok1 && ok2
}

func unhexInternal(c byte) (byte, bool) {
	switch {
	case c >= '0' && c <= '9':
		return c - '0', true
	case c >= 'a' && c <= 'f':
		return c - 'a' + 10, true
	case c >= 'A' && c <= 'F':
		return c - 'A' + 10, true
	}
	return 0, false
}
