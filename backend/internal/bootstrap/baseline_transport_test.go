// Source: docklet-next/docs/design/updates-ui.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original proof that drift statuses cross real Direct and Edge transports.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// legacyDriftBody is the exact answer of agents before M35 after an external
// env change; fixedDriftBody carries current revisions.
const legacyDriftBody = `{"success":true,"data":{"targetIdentity":"f155c7cd0819990dc51c6b1ee8c8ee0819d8ee4076560b91f873460fae7cff16","autoExcluded":false,"savedRevision":"","observedRevision":"","available":false,"code":"update.config-drift"}}`

func fixedDriftBody() string {
	b, _ := json.Marshal(base.ApiResponse[dto.BaselineStatus]{Success: true, Data: dto.BaselineStatus{TargetIdentity: strings.Repeat("a", 64), SavedRevision: strings.Repeat("b", 64), ObservedRevision: strings.Repeat("c", 64), Code: "update.config-drift"}})
	return string(b)
}

func TestDriftBaselineCrossesRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for name, body := range map[string]string{"legacy": legacyDriftBody, "fixed": fixedDriftBody()} {
			t.Run(mode+"/"+name, func(t *testing.T) {
				x := newM15Transport(t, mode)
				x.baselineBody.Store(body)
				// Browser path: the hub forwards the agent's bytes unchanged.
				r := httptest.NewRequest(http.MethodGet, "/api/environments/example/updater/projects/project-uuid/baseline", nil)
				r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: x.fixture.tokens.BrowserToken})
				w := httptest.NewRecorder()
				x.hub.ServeHTTP(w, r)
				require.Equal(t, 200, w.Code, w.Body.String())
				require.JSONEq(t, body, w.Body.String())
				// Hub policy path: the environment proxy plus the executor envelope check.
				var out base.ApiResponse[dto.BaselineStatus]
				require.NoError(t, x.hosts.ProxyJSONRequest(t.Context(), "example", http.MethodGet, "/api/environments/0/updater/projects/project-uuid/baseline", nil, &out))
				require.True(t, out.Success)
				require.True(t, out.Data.Valid(), "the policy validator accepts the drift status")
				require.NoError(t, imageDTO.ValidateReason(out.Data.Code))
				require.False(t, out.Data.Available)
				require.Equal(t, "update.config-drift", out.Data.Code)
			})
		}
	}
}
