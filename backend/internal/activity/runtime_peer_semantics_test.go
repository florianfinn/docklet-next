// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	a "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

func TestS4AcknowledgementCanonicalSemantics(t *testing.T) {
	f := b1Fixture(t)
	for _, op := range []RuntimeOperation{RuntimeDeployProject, RuntimeRedeployProject, RuntimeDownProject, RuntimeStopProject, RuntimeRestartProject, RuntimeUpdateProject, RuntimeRestoreProject} {
		t.Run(string(op), func(t *testing.T) {
			c := f.command()
			c.Operation = op
			if runtimeImageOperation(op) {
				c = imageCommand(c, op)
			}
			plain, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			legacyGolden := map[RuntimeOperation]string{RuntimeOperation("deploy-project"): "98d48fcec2f31ab6cd111a1788a15b10c8e82a0b85d648c23626d116734a0658", RuntimeOperation("redeploy-project"): "64b804ed24a4781426e9c92c1471672d59ff0dff467bd9d4458b57b67f8bdf1f", RuntimeOperation("down-project"): "639622b73e28ddfb13b234bf23b21279bbf860470d182aecd99d8c5363b6c7fb", RuntimeOperation("stop-project"): "ad14d9ae3158ccea1afb3307cea91f3d2cfcabeeeeb72f3a56bc65abef915dd8", RuntimeOperation("restart-project"): "9d282f4de33768d05bc7a981f71e410ddfa2b5d2f9e281e91c54f9c2608cdbbc"}
			if golden := legacyGolden[op]; golden != "" {
				require.Equal(t, golden, plain.digest, "empty acknowledgement preserves the frozen pre-v20 digest")
			}
			// Empty acknowledgement retains the pre-v20 five-field semantic encoding.
			var options any = [0]string{}
			if plain.command.Options.Lifecycle != nil {
				options = plain.command.Options.Lifecycle
			} else if plain.command.Options.Deploy != nil {
				options = [4]any{"", false, false, false}
			} else if op == RuntimeRestartProject {
				options = []string{}
			}
			require.Equal(t, runtimeDigestInternal([5]any{string(op), c.Target.Kind, c.Target.ID, options, c.Target.ParentProjectID}), plain.digest, "legacy digest must stay byte identical")
			c.Options.AcknowledgedUncertainty = strings.Repeat("AB", 32)
			confirmed, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.Equal(t, strings.Repeat("ab", 32), confirmed.command.Options.AcknowledgedUncertainty, "every canonicalizer branch preserves acknowledgement")
			require.NotEqual(t, plain.digest, confirmed.digest, "confirmed intent cannot reuse an unconfirmed receipt")
			require.NotEqual(t, plain.gateKey, confirmed.gateKey, "confirmed intent cannot join an unconfirmed run")
			c.Options.AcknowledgedUncertainty = strings.Repeat("ab", 32)
			same, err := runtimeCanonicalizeInternal(c)
			require.NoError(t, err)
			require.Equal(t, confirmed.digest, same.digest)
			c.Options.AcknowledgedUncertainty = "invalid"
			_, err = runtimeCanonicalizeInternal(c)
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "runtime.invalid-acknowledgement", problem.Problem.Code)
		})
	}
}

func TestS4DurableAcknowledgementAndActualUseAudit(t *testing.T) {
	for _, needed := range []bool{false, true} {
		t.Run(map[bool]string{false: "unused", true: "used"}[needed], func(t *testing.T) {
			f := b1Fixture(t)
			digest := strings.Repeat("a", 64)
			used := &imageDTO.RuntimePeerOverrides{Count: 1, Peers: []imageDTO.RuntimeIdentityPeer{{PeerID: "example-peer", Name: "example-peer", Reason: "compose.identity-unreadable"}}}
			f.service.resolver = RuntimeAcknowledgedResolverFunc(func(_ context.Context, target RuntimeTarget, ack string) (RuntimeResolvedTarget, error) {
				require.Equal(t, digest, ack, "admission receives canonical acknowledgement")
				result := RuntimeResolvedTarget{Target: target}
				if needed {
					result.PeerOverrides = used
				}
				return result, nil
			})
			c := f.command()
			c.Options.AcknowledgedUncertainty = digest
			result, err := f.service.Admit(t.Context(), c)
			require.NoError(t, err)
			var row RuntimeReceipt
			require.NoError(t, f.store.svc.db.First(&row, "run_id = ?", result.ActivityID).Error)
			require.Equal(t, digest, row.AcknowledgedUncertainty, "receipt owns acknowledgement independently of history")
			var messages []ActivityMessage
			require.NoError(t, f.store.svc.db.Where("activity_id = ? AND code = ?", result.ActivityID, "runtime.peer-acknowledged").Find(&messages).Error)
			if needed {
				require.Len(t, messages, 1)
				require.Contains(t, row.PeerOverrides, "example-peer")
			} else {
				require.Empty(t, messages, "unused supplied acknowledgements must not fabricate an override event")
				require.JSONEq(t, "{}", row.PeerOverrides)
			}
			// A new service owner reads the durable request without consulting identity I/O.
			reopened := NewRuntimeService(f.store.svc, nil)
			retained, found, err := reopened.RetainedCommand(t.Context(), c)
			require.NoError(t, err)
			require.True(t, found)
			require.Equal(t, digest, retained.Options.AcknowledgedUncertainty)
			if needed {
				require.Equal(t, used, retained.PeerOverrides)
			}
			c.Options.AcknowledgedUncertainty = ""
			_, _, err = reopened.RetainedCommand(t.Context(), c)
			require.Error(t, err, "retained callers cannot inherit another intent's confirmation")
			c.Key = uuid.NewString()
			_, err = f.service.Admit(t.Context(), c)
			var busy *GateConflict
			require.ErrorAs(t, err, &busy, "different confirmation semantics cannot join")
		})
	}
}

func TestS4UpdateAdmissionPreservesConfirmationAndLifecycle(t *testing.T) {
	dispatched := make(chan RuntimeCommand, 1)
	f := b2Fixture(t, t.Context(), func(_ context.Context, c RuntimeCommand, _ io.Writer) error { dispatched <- c; return nil })
	digest := strings.Repeat("a", 64)
	request := dto.LifecycleRequest{ExpectedRevision: strings.Repeat("a", 64), Service: "app", ExpectedManifest: "sha256:" + strings.Repeat("e", 64)}
	resolve := RuntimeAcknowledgedResolverFunc(func(_ context.Context, target RuntimeTarget, ack string) (RuntimeResolvedTarget, error) {
		if ack != digest {
			problem := imageDTO.Failure("runtime.target-unavailable", 409)
			return RuntimeResolvedTarget{}, problem
		}
		return RuntimeResolvedTarget{Target: target}, nil
	})
	f.service.resolver = resolve
	prepare := func(ctx context.Context, _ string, target RuntimeTarget, ack string) (RuntimeResolvedTarget, error) {
		return resolve.ResolveAcknowledgedRuntimeTarget(ctx, target, ack)
	}
	var prepared dto.LifecycleRequest
	h, err := NewRuntimeHTTPService(f.service, RuntimeHTTPPorts{ApplicationContext: t.Context(), Store: func(context.Context, string) (RuntimeStoreBinding, error) { return f.command().Store, nil }, PrepareTarget: func(ctx context.Context, _ string, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		return resolve.ResolveRuntimeTarget(ctx, target)
	}, PrepareAcknowledgedTarget: prepare, PrepareLifecycle: func(context.Context, string, RuntimeTarget, RuntimeStoreBinding, dto.LifecycleRequest) (*dto.LifecycleIntent, error) {
		t.Fatal("confirmed lifecycle used legacy preparation")
		return nil, nil
	}, PrepareAcknowledgedLifecycle: func(_ context.Context, _ string, _ RuntimeTarget, _ RuntimeStoreBinding, r dto.LifecycleRequest, ack string) (*dto.LifecycleIntent, error) {
		require.Equal(t, digest, ack)
		prepared = r
		intent := imageCommand(f.command(), RuntimeUpdateProject).Options.Lifecycle
		intent.Request = r
		return intent, nil
	}})
	require.NoError(t, err)
	h.SetReady(true)
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("runtime", "test"))
	api.UseMiddleware(func(ctx huma.Context, next func(huma.Context)) {
		ctx2 := context.WithValue(ctx.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		actor := f.command().Actor
		ctx2 = context.WithValue(ctx2, usertypes.CurrentUserContextKey{}, &actor)
		next(huma.WithContext(ctx, ctx2))
	})
	RegisterImageLifecycle(api, f.store.svc)
	body, err := json.Marshal(a.RuntimeLifecycleAdmissionRequest{LifecycleRequest: request, RuntimePeerAdmissionRequest: a.RuntimePeerAdmissionRequest{AcknowledgedUncertainty: digest}})
	require.NoError(t, err)
	req := httptest.NewRequest(http.MethodPost, "/environments/0/updater/projects/"+f.command().Target.ID+"/update", strings.NewReader(string(body)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Prefer", "respond-async")
	req.Header.Set("Idempotency-Key", uuid.NewString())
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	require.Equal(t, 202, rec.Code, rec.Body.String())
	require.Equal(t, request, prepared, "update retains the original expected revision, service and manifest through preparation")
	var response struct{ Data a.RuntimeAdmission }
	require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &response))
	settled, err := b2Wait(t, f, f.command(), response.Data.ActivityID)
	require.NoError(t, err)
	require.Equal(t, a.StatusSuccess, settled.Status)
	c := <-dispatched
	require.Equal(t, digest, c.Options.AcknowledgedUncertainty, "update acknowledgement reaches dispatch")
	require.Equal(t, request, c.Options.Lifecycle.Request, "verified update intent reaches dispatch unchanged")
}
