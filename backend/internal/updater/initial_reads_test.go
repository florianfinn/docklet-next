// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Fresh policy identity and malformed executor response proofs.

package updater

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/testfixtures"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/types/base"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
)

func TestUnsetPolicyRead(t *testing.T) {
	for _, env := range []string{"0", "edge-example"} {
		t.Run(env, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.baseline.Available = false
			f.baseline.Code = "update.baseline-unavailable"
			router := echo.New()
			api := humaecho.New(router, huma.DefaultConfig("test", "test"))
			registerImagePolicyRoutes(api, f.s)
			request := httptest.NewRequest(http.MethodGet, "/environments/"+env+"/updater/policies/project/example-stack", http.NoBody)
			ctx := context.WithValue(request.Context(), middleware.ContextKeyUserPermissions, &authz.PermissionSet{Sudo: true})
			for i := 0; i < 2; i++ {
				response := httptest.NewRecorder()
				router.ServeHTTP(response, request.WithContext(ctx))
				require.Equal(t, 200, response.Code, response.Body.String())
				var envelope struct{ Data dto.AutoUpdatePolicy }
				require.NoError(t, json.Unmarshal(response.Body.Bytes(), &envelope))
				if env == "0" {
					fixture, e := os.ReadFile("../../../web/src/data/http/testdata/unset-policy.json")
					require.NoError(t, e)
					testfixtures.Read(t, "policy", envelope.Data)
					actual, e := json.Marshal(envelope.Data)
					require.NoError(t, e)
					require.JSONEq(t, string(fixture), string(actual))
				}
				require.Equal(t, f.baseline.TargetIdentity, envelope.Data.TargetIdentity)
				require.Zero(t, envelope.Data.Revision)
				require.Equal(t, "off", envelope.Data.Mode)
				require.Empty(t, envelope.Data.Candidates)
				require.NotNil(t, envelope.Data.Candidates)
				require.Nil(t, envelope.Data.CheckedAt)
			}
			var count int64
			require.NoError(t, f.db.Model(&policy.Policy{}).Count(&count).Error)
			require.Zero(t, count)
			require.Zero(t, f.admissions)
			require.Zero(t, f.checks)
			require.Zero(t, f.notifications)
			saved, e := f.s.SaveImagePolicy(t.Context(), env, "example-stack", dto.UpdatePolicyInput{Mode: "notify", ExpectedRevision: 0}, "example-actor", "")
			require.NoError(t, e)
			require.EqualValues(t, 1, saved.Revision)
		})
	}
}
func TestUnsetPolicyRemoteDecode(t *testing.T) {
	for _, field := range []string{"identity", "saved", "observed", "transport"} {
		t.Run(field, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) {
				value := f.baseline
				switch field {
				case "identity":
					value.TargetIdentity = strings.Repeat("z", 64)
				case "saved":
					value.SavedRevision = ""
				case "observed":
					value.ObservedRevision = "invalid"
				case "transport":
					return dto.BaselineStatus{}, context.Canceled
				}
				return value, nil
			}
			_, e := f.s.GetImagePolicy(t.Context(), "0", "stack")
			require.Error(t, e)
			require.Equal(t, "update.check-unavailable", policyCode(e, ""))
			_, e = f.s.SaveImagePolicy(t.Context(), "0", "stack", dto.UpdatePolicyInput{Mode: "notify"}, "example-actor", "")
			require.Error(t, e)
			var count int64
			require.NoError(t, f.db.Model(&policy.Policy{}).Count(&count).Error)
			require.Zero(t, count)
		})
	}
}

func TestUnsetScheduleRead(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Where("1 = 1").Delete(&policy.HostSchedule{}).Error)
	row, e := f.s.ImagePolicySchedule(t.Context(), "0")
	require.NoError(t, e)
	require.Zero(t, row.Revision)
	require.NotEmpty(t, row.RegistrationID)
	require.Equal(t, "0 0 * * * *", row.PollingInterval)
	require.Equal(t, "0 0 0 * * *", row.AutoUpdateInterval)
	require.Empty(t, row.ActorID)
	var count int64
	require.NoError(t, f.db.Model(&policy.HostSchedule{}).Count(&count).Error)
	require.Zero(t, count)
	require.Zero(t, f.admissions)
	require.Zero(t, f.checks)
}

func TestUnsetPolicyBaselineEnvelope(t *testing.T) {
	value := dto.BaselineStatus{TargetIdentity: strings.Repeat("a", 64), SavedRevision: strings.Repeat("b", 64), ObservedRevision: strings.Repeat("c", 64)}
	for _, success := range []bool{false, true} {
		out := base.ApiResponse[dto.BaselineStatus]{Success: success, Data: value}
		_, err := checkedBaselineResponse(out)
		if success {
			require.NoError(t, err)
		} else {
			require.Error(t, err)
		}
	}
	for _, field := range []string{"targetIdentity", "savedRevision", "observedRevision"} {
		for _, bad := range []any{nil, "", strings.Repeat("z", 64), 42} {
			body, e := json.Marshal(value)
			require.NoError(t, e)
			var data map[string]any
			require.NoError(t, json.Unmarshal(body, &data))
			data[field] = bad
			encoded, e := json.Marshal(map[string]any{"success": true, "data": data})
			require.NoError(t, e)
			var out base.ApiResponse[dto.BaselineStatus]
			if json.Unmarshal(encoded, &out) == nil {
				_, err := checkedBaselineResponse(out)
				require.Error(t, err, "%s=%v", field, bad)
			}
		}
	}
}
