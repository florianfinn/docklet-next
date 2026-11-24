// Source: docklet-next/issues/594
// Revision: 48d8f45d867a7a41e571b59382b291c350d3ce42
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original eligibility proxy permission conjunction regression.

package middleware

import (
	"context"
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestEligibilityProxyNeedsBothBaselineReadGrants(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, missing := range []string{authz.PermProjectsRead, authz.PermImageUpdatesRead} {
			ps := &authz.PermissionSet{Global: map[string]struct{}{authz.PermProjectsRead: {}, authz.PermImageUpdatesRead: {}}}
			delete(ps.Global, missing)
			body := []byte(`{"success":true,"data":{"services":[]}}`)
			hold, dbErr, deletes := dto.HoldNone, false, 0
			router := pointProxyAs(t, isEdge, &body, &hold, &dbErr, &deletes, ps, func(context.Context, string, string, string) error { return nil })
			rec := pointCall(router, http.MethodGet, "/updater/projects/stack/update-eligibility")
			require.Equal(t, 403, rec.Code, rec.Body.String())
		}
	}
}
