// Source: docklet-next/issues/492
// Revision: a00b3f23b9cb0eaa0fa2ca0cb58cbfaa397f1a2b
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original M28 resolver ingress redaction; follows widget.ErrorTransformer.

package proxy

import (
	"regexp"
	"strconv"

	"github.com/danielgtaylor/huma/v2"
	dto "github.com/florianfinn/docklet-next/types/proxy"
)

var resolverPath = regexp.MustCompile(`^(/api)?(/modules/proxy/instances/[^/]+|/environments/[^/]+/modules/proxy)/resolvers/?$`)

// ErrorTransformer replaces Huma's own error bodies on resolver routes: their
// validation details echo submitted values, which may be provider credentials.
func ErrorTransformer(ctx huma.Context, status string, value any) (any, error) {
	if !resolverPath.MatchString(ctx.URL().Path) {
		return value, nil
	}
	n, _ := strconv.Atoi(status)
	// Body errors (400, 413, 415, 422, ...) may carry the raw body; auth, gate
	// and conflict answers carry none and keep their meaning.
	if _, ok := value.(*huma.ErrorModel); !ok || n < 400 || n > 499 || n == 401 || n == 403 || n == 404 || n == 409 {
		return value, nil
	}
	ctx.SetStatus(422)
	return dto.Fail(dto.InvalidInput, 422), nil
}
