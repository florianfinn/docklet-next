// Source: docklet-next/issues/322
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original simple-user resource authorization.

package access

import (
	"encoding/json"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/types/user"
)

func Problem(status int, code string) *user.AccessProblem {
	return &user.AccessProblem{Code: user.AccessCode(code), Status: status, Detail: code}
}
func WriteProblem(ctx huma.Context, status int, code string) error {
	ctx.SetHeader("Content-Type", "application/problem+json")
	ctx.SetStatus(status)
	return json.NewEncoder(ctx.BodyWriter()).Encode(Problem(status, code))
}
