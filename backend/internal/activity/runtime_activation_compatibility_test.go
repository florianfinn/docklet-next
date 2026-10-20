// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Registered legacy status, permission and retained-key boundaries.

package activity

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func activationLegacyMux(t *testing.T, f *b1FixtureInternal, c RuntimeCommand, ps *authz.PermissionSet) (http.Handler, string) {
	t.Helper()
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) {
		t.Fatal("retained retry read current executor")
		return RuntimeStoreBinding{}, nil
	}, PrepareTarget: func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		t.Fatal("retained retry inspected Docker")
		return RuntimeResolvedTarget{}, nil
	}})
	require.NoError(t, err)
	h.SetReady(true)
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Legacy runtime", "0.0.0"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		request := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, ps)
		request = context.WithValue(request, usertypes.CurrentUserContextKey{}, &c.Actor)
		next(huma.WithContext(ctx, request))
	})
	path := "/environments/{id}/projects/{projectId}/operation"
	target := "/environments/" + c.Store.EnvironmentID + "/projects/" + c.Target.ID + "/operation"
	if c.Target.Kind == "container" {
		path = "/environments/{id}/containers/{containerId}/operation"
		target = "/environments/" + c.Store.EnvironmentID + "/containers/" + c.Target.ID + "/operation"
	}
	RegisterRuntimeLifecycle(api, f.store.svc, c.Operation, huma.Operation{OperationID: "legacy-runtime", Method: http.MethodPost, Path: path})
	return mux, target
}

func activationLegacyPermission(c RuntimeCommand) *authz.PermissionSet {
	ps := authz.NewPermissionSet()
	ps.AddEnv(c.Store.EnvironmentID, runtimePermissionInternal(c.Operation), authz.PermActivitiesRead)
	return ps
}

func activationLegacyRequest(t *testing.T, mux http.Handler, path, key, prefer string) *httptest.ResponseRecorder {
	t.Helper()
	request := httptest.NewRequest(http.MethodPost, path, nil)
	request.Header.Set("Idempotency-Key", key)
	request.Header.Set("Prefer", prefer)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, request)
	return rec
}

func TestRuntimeActivationLegacyTerminalResponses(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeDeployProject, RuntimeDownProject, RuntimeRestartProject, RuntimeRedeployProject, RuntimeStartContainer, RuntimeStopContainer, RuntimeRestartContainer} {
		for _, tc := range []struct {
			name, state string
			code        activitytypes.Code
			status      int
		}{
			{"success", "success", activitytypes.CodeActivitySuccess, 200},
			{"failed", "failed", activitytypes.CodeRuntimeFailed, 500},
			{"cancelled", "cancelled", activitytypes.CodeRuntimeCancelled, 409},
			{"interrupted", "failed", activitytypes.CodeRuntimeInterrupted, 409},
		} {
			t.Run(string(op)+"/"+tc.name, func(t *testing.T) {
				f := b1Fixture(t)
				c := f.command()
				c.Operation = op
				if strings.HasSuffix(string(op), "container") {
					c.Target = RuntimeTarget{Kind: "container", ID: "example-container"}
				}
				admitted, err := f.service.Admit(t.Context(), c)
				require.NoError(t, err)
				canonical, err := runtimeCanonicalizeInternal(c)
				require.NoError(t, err)
				id := canonical.identity(admitted.ActivityID)
				ctx := context.WithValue(t.Context(), gateRunContextKey{}, id.RunID)
				require.NoError(t, f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
					if _, err := runtimeLocalAcceptTxInternal(tx, now, id); err != nil {
						return err
					}
					if _, err := runtimeLocalExecutingTxInternal(tx, now, id); err != nil {
						return err
					}
					_, _, err := runtimeCompleteTxInternal(tx, now, id, runtimeLocalResultInternal{state: tc.state, code: tc.code, message: string(tc.code), settledAt: now, startedAt: new(now)})
					return err
				}))
				mux, path := activationLegacyMux(t, f, c, activationLegacyPermission(c))
				response := activationLegacyRequest(t, mux, path, c.Key, "")
				body := response.Body.String()
				stream := op == RuntimeDeployProject || op == RuntimeRedeployProject
				if stream {
					require.Equal(t, 200, response.Code, body)
					require.Contains(t, body, `"activityId":"`+id.RunID+`"`)
					if tc.status == 200 {
						require.Contains(t, body, `{"done":true}`)
						require.NotContains(t, body, `"error"`)
					} else {
						require.NotContains(t, body, `{"done":true}`)
						require.Contains(t, body, `"code":"`+string(tc.code)+`"`)
					}
				} else {
					require.Equal(t, tc.status, response.Code, body)
					require.Contains(t, body, `"activityId":"`+id.RunID+`"`)
					if tc.status == 200 {
						require.Contains(t, body, `"success":true`)
						require.Contains(t, body, `"message":`)
					} else {
						require.Contains(t, body, `"success":false`)
						require.Contains(t, body, `"code":"`+string(tc.code)+`"`)
					}
				}
				retry := activationLegacyRequest(t, mux, path, c.Key, "respond-async")
				require.Equal(t, 200, retry.Code, retry.Body.String())
				var envelope struct {
					Data activitytypes.RuntimeAdmission `json:"data"`
				}
				require.NoError(t, json.Unmarshal(retry.Body.Bytes(), &envelope))
				require.Equal(t, id.RunID, envelope.Data.ActivityID)
				require.Equal(t, id.RunID, envelope.Data.Activity.ID)
			})
		}
	}
}

func TestRuntimeActivationRetainedBoundaryCallerAndPermission(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	c.Operation = RuntimeStartContainer
	c.Target = RuntimeTarget{Kind: "container", ID: "example-container", ParentProjectID: new("example-stack")}
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	for _, mode := range []string{"missing-read", "missing-operation", "other-caller", "different-intent"} {
		t.Run(mode, func(t *testing.T) {
			request := c
			request.Target.ParentProjectID = nil
			ps := activationLegacyPermission(c)
			if mode == "missing-read" {
				ps = authz.NewPermissionSet()
				ps.AddEnv("0", runtimePermissionInternal(c.Operation))
			}
			if mode == "missing-operation" {
				ps = authz.NewPermissionSet()
				ps.AddEnv("0", authz.PermActivitiesRead)
			}
			if mode == "other-caller" {
				request.Actor.ID = "another-caller"
			}
			if mode == "different-intent" {
				request.Operation = RuntimeStopContainer
				ps = activationLegacyPermission(request)
			}
			mux, path := activationLegacyMux(t, f, request, ps)
			response := activationLegacyRequest(t, mux, path, c.Key, "respond-async")
			if strings.HasPrefix(mode, "missing-") {
				require.Equal(t, 403, response.Code)
			} else {
				require.Equal(t, 409, response.Code)
				require.Contains(t, response.Body.String(), string(activitytypes.CodeRunRequestMismatch))
			}
			require.NotContains(t, response.Body.String(), admitted.ActivityID)
		})
	}
}

func TestRuntimeActivationUnsupportedPullPolicyBeforeAdmission(t *testing.T) {
	f := b1Fixture(t)
	c := f.command()
	mux, path := activationLegacyMux(t, f, c, activationLegacyPermission(c))
	ports := 0
	f.service.activity.runtimeHTTP.ports.Store = func(context.Context, string) (RuntimeStoreBinding, error) {
		ports++
		return RuntimeStoreBinding{}, ErrRuntimeTransport
	}
	f.service.activity.runtimeHTTP.ports.PrepareTarget = func(context.Context, string, RuntimeTarget) (RuntimeResolvedTarget, error) {
		ports++
		return RuntimeResolvedTarget{}, ErrRuntimeTransport
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewBufferString(`{"pullPolicy":"unsupported"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	response := httptest.NewRecorder()
	mux.ServeHTTP(response, req)
	require.Equal(t, 400, response.Code, response.Body.String())
	require.Zero(t, ports, "unsupported policy must fail before any executor metadata port")
	require.Zero(t, f.resolver.calls.Load())
	require.Empty(t, runtimeBoundarySnapshot(t, f.store).Receipts)
}
