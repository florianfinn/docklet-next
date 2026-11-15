// Source: docklet-next/issues/394
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original legacy dashboard volume counts caller regression for issue #394.

package dashboard

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/contract"
	"github.com/florianfinn/docklet-next/types/version"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type legacyCountsTransport func(*http.Request) (*http.Response, error)

func (f legacyCountsTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// The legacy snapshot reads volume counts from the collection route; a volume named "counts" answers
// only on its inspect path, as a current agent routes it.
func TestLegacyDashboardReadsVolumeCountsFromCollectionRoute(t *testing.T) {
	for _, named := range []bool{false, true} {
		t.Run(map[bool]string{false: "plain", true: "volume-named-counts"}[named], func(t *testing.T) {
			raw, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
			require.NoError(t, err)
			require.NoError(t, raw.AutoMigrate(&environment.Environment{}))
			pool, err := raw.DB()
			require.NoError(t, err)
			pool.SetMaxOpenConns(1)
			t.Cleanup(func() { require.NoError(t, pool.Close()) })
			row := environment.Environment{
				BaseModel: database.BaseModel{ID: "example-host"}, Name: "Example", Enabled: true,
				ApiUrl: "http://agent.example.com", AccessToken: new("example-token"),
			}
			require.NoError(t, raw.Create(&row).Error)
			versionBody, err := json.Marshal(version.Info{AgentContractVersion: contract.AgentHubVersion, AgentContractFingerprint: contract.AgentHubFingerprint})
			require.NoError(t, err)
			client := &http.Client{Transport: legacyCountsTransport(func(r *http.Request) (*http.Response, error) {
				status, body := http.StatusNotFound, `{"code":"not-found"}`
				switch r.URL.Path {
				case "/api/app-version":
					status, body = http.StatusOK, string(versionBody)
				case "/api/environments/0/system/health":
					status, body = http.StatusNoContent, ""
				case "/api/environments/0/volumes/_collection/counts":
					status, body = http.StatusOK, `{"success":true,"data":{"inuse":2,"unused":3,"total":5}}`
				case "/api/environments/0/volumes/counts":
					if named {
						status, body = http.StatusOK, `{"success":true,"data":{"id":"counts","name":"counts","driver":"local","inUse":false}}`
					}
				}
				return &http.Response{StatusCode: status, Header: http.Header{"Content-Type": {"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: r}, nil
			})}
			handler := &DashboardHandler{environmentService: environment.NewEnvironmentService(&database.DB{DB: raw}, client, nil, nil, nil, nil)}
			snapshot, err := handler.fetchLegacyDashboardSnapshotInternal(t.Context(), row)
			require.NoError(t, err)
			require.Equal(t, &volumetypes.UsageCounts{Inuse: 2, Unused: 3, Total: 5}, snapshot.VolumeUsageCounts)
		})
	}
}
