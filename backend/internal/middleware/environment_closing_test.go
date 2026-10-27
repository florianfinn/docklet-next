// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Direct/Edge lifecycle response and point authority regression proofs for #229.

package middleware

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

func TestClosingLifecycleProxyErrors(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, route := range []struct{ method, path string }{
			{"GET", "/updater/restore-points"}, {"DELETE", "/updater/restore-points/point"},
			{"GET", "/updater/projects/stack/baseline"},
		} {
			for _, tc := range []struct {
				name, code, want   string
				status, wantStatus int
			}{
				{"undeclared", "undeclared", "runtime.outcome-unknown", 409, 503},
				{"wrong-enum", "job.skipped", "runtime.outcome-unknown", 409, 503},
				{"unauthorized", "restore.unavailable", "agent.invalid-token", 401, 502},
				{"forbidden", "restore.unavailable", "agent.invalid-token", 403, 502},
				{"declared", "restore.unavailable", "restore.unavailable", 409, 409},
				{"redirect", "restore.unavailable", "runtime.outcome-unknown", 307, 503},
			} {
				t.Run(fmt.Sprintf("edge=%t/%s/%s/%s", isEdge, route.method, route.path, tc.name), func(t *testing.T) {
					problem := imageDTO.Failure(tc.code, tc.status)
					problem.Problem.Message = "synthetic-private diagnostic"
					body, err := json.Marshal(problem)
					require.NoError(t, err)
					hold, dbErr, deletes := dto.HoldNone, false, 0
					rec := pointCall(pointProxy(t, isEdge, &body, &hold, &dbErr, &deletes, tc.status), route.method, route.path)
					require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
					require.Contains(t, rec.Body.String(), `"code":"`+tc.want+`"`)
					require.NotContains(t, rec.Body.String(), "synthetic-private")
				})
			}
		}
	}
}

func TestClosingRemotePointAuthority(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, admin := range []bool{false, true} {
			for _, orphan := range []bool{false, true} {
				t.Run(fmt.Sprintf("edge=%t/admin=%t/orphan=%t", isEdge, admin, orphan), func(t *testing.T) {
					ps := authz.EnvironmentPermissionSet("example")
					if admin {
						ps = authz.SudoPermissionSet()
					}
					body := pointBody(t)
					if orphan {
						body = bytes.Replace(body, []byte(`"orphaned":false`), []byte(`"orphaned":true`), 1)
					}
					hold, dbErr, deletes, checks := dto.HoldNone, false, 0, 0
					access := func(_ context.Context, env, target, point string) error {
						checks++
						require.Equal(t, "example", env)
						require.Equal(t, "stack", target)
						if orphan {
							return imageDTO.Failure("update.invalid-target", 400)
						}
						return nil
					}
					router := pointProxyAs(t, isEdge, &body, &hold, &dbErr, &deletes, ps, access)
					for _, method := range []string{"GET", "DELETE"} {
						path := "/updater/project/stack/restore-points"
						if method == "DELETE" {
							path += "/point"
						}
						rec := pointCall(router, method, path)
						if orphan && !admin {
							require.Equal(t, 400, rec.Code)
							require.Contains(t, rec.Body.String(), "update.invalid-target")
						} else if method == "GET" {
							require.Equal(t, 200, rec.Code)
						} else {
							require.Equal(t, 204, rec.Code)
						}
					}
					if orphan && !admin {
						require.Zero(t, deletes)
					} else {
						require.Equal(t, 1, deletes)
					}
					if admin {
						require.Zero(t, checks)
					} else {
						require.Equal(t, 2, checks)
					}
				})
			}
		}
	}
}

func TestClosingPointBecomesOrphanAfterAuthorityRead(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		body := bytes.Replace(pointBody(t), []byte(`"orphaned":false`), []byte(`"orphaned":true`), 1)
		hold, dbErr, deletes := dto.HoldNone, false, 0
		router := pointProxyAs(t, isEdge, &body, &hold, &dbErr, &deletes, authz.EnvironmentPermissionSet("example"), func(context.Context, string, string, string) error { return nil })
		rec := pointCall(router, http.MethodGet, "/updater/project/stack/restore-points")
		require.Equal(t, 400, rec.Code)
		require.Contains(t, rec.Body.String(), "update.invalid-target")
	}
}

func TestClosingLifecycleSuccessEnvelope(t *testing.T) {
	for _, isEdge := range []bool{false, true} {
		for _, tc := range []struct {
			name, body         string
			status, wantStatus int
			want               string
		}{
			{"failed-success-status", `{"success":false,"error":{"code":"undeclared","message":"synthetic-private"}}`, 200, 503, "runtime.outcome-unknown"},
			{"undeclared-status", `{"success":true,"data":{}}`, 201, 503, "update.check-unavailable"},
			{"missing-envelope", `{}`, 200, 503, "update.check-unavailable"},
			{"baseline-unknown", `{"success":true,"data":{"code":"undeclared"}}`, 200, 503, "runtime.outcome-unknown"},
			{"baseline-malformed", `{"success":true,"data":{"code":7}}`, 200, 503, "runtime.outcome-unknown"},
			{"baseline-declared", `{"success":true,"data":{"code":"update.baseline-unavailable"}}`, 200, 200, "update.baseline-unavailable"},
		} {
			t.Run(fmt.Sprintf("edge=%t/%s", isEdge, tc.name), func(t *testing.T) {
				body := []byte(tc.body)
				hold, dbErr, deletes := dto.HoldNone, false, 0
				rec := pointCall(pointProxy(t, isEdge, &body, &hold, &dbErr, &deletes, tc.status), "GET", "/updater/projects/stack/baseline")
				require.Equal(t, tc.wantStatus, rec.Code, rec.Body.String())
				require.Contains(t, rec.Body.String(), tc.want)
				require.NotContains(t, rec.Body.String(), "synthetic-private")
			})
		}
		body := []byte(`{"success":false,"error":{"code":"undeclared","message":"synthetic-private"}}`)
		hold, dbErr, deletes := dto.HoldNone, false, 0
		rec := pointCall(pointProxy(t, isEdge, &body, &hold, &dbErr, &deletes, 200), "DELETE", "/updater/restore-points/point")
		require.Equal(t, 503, rec.Code)
		require.Contains(t, rec.Body.String(), "runtime.outcome-unknown")
		require.NotContains(t, rec.Body.String(), "synthetic-private")
	}
}
