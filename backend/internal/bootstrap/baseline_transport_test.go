// Source: docklet-next/docs/design/updates-ui.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original proof that drift statuses cross real Direct and Edge transports unchanged.

package bootstrap

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/utils/cookie"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// The shared fixture holds the exact pre-M35 drift body and the producer's current shape (bound by the
// project producer test); the web decoder test reads the same file.
func driftBodies(t *testing.T) map[string]json.RawMessage {
	t.Helper()
	body, err := os.ReadFile("../../../types/updater/testdata/drift-baseline.json")
	require.NoError(t, err)
	var out map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(body, &out))
	return out
}

func TestDriftBaselineCrossesRealDirectAndEdge(t *testing.T) {
	for _, mode := range []string{"direct", "edge"} {
		for name, body := range driftBodies(t) {
			t.Run(mode+"/"+name, func(t *testing.T) {
				x := newM15Transport(t, mode)
				x.baselineBody.Store(string(body))
				var sent base.ApiResponse[dto.BaselineStatus]
				require.NoError(t, json.Unmarshal(body, &sent))
				browser := func(path string) *httptest.ResponseRecorder {
					r := httptest.NewRequest(http.MethodGet, path, nil)
					r.AddCookie(&http.Cookie{Name: cookie.InsecureTokenCookieName, Value: x.fixture.tokens.BrowserToken})
					w := httptest.NewRecorder()
					x.hub.ServeHTTP(w, r)
					require.Equal(t, 200, w.Code, w.Body.String())
					return w
				}
				// Browser path: the hub forwards the agent's JSON unchanged to the web decoder.
				require.JSONEq(t, string(body), browser("/api/environments/example/updater/projects/project-uuid/baseline").Body.String())
				// Hub path: the environment proxy hands the policy consumer (updater TestDriftFixturePolicyConsumer)
				// exactly the fixture's envelope.
				var out base.ApiResponse[dto.BaselineStatus]
				require.NoError(t, x.hosts.ProxyJSONRequest(t.Context(), "example", http.MethodGet, "/api/environments/0/updater/projects/project-uuid/baseline", nil, &out))
				require.Equal(t, sent, out)
			})
		}
	}
}
