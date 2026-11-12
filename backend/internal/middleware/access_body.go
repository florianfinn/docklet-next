// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package middleware

import (
	"bytes"
	"github.com/labstack/echo/v5"
	"io"
)

func readAccessBody(c *echo.Context) ([]byte, error) {
	r := c.Request()
	if r.Body == nil {
		return nil, nil
	}
	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	r.Body = io.NopCloser(bytes.NewReader(body))
	return body, err
}
