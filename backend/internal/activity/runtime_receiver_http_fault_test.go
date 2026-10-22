// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"
	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type runtimeNonAgentResolver struct{ credential string }

func (r runtimeNonAgentResolver) ResolveEnvironmentByAccessToken(_ context.Context, token string) (*environment.Environment, error) {
	if token != r.credential {
		return nil, errors.New("invalid synthetic credential")
	}
	return &environment.Environment{ID: "0", Name: "Example target", AccessToken: new(r.credential)}, nil
}
func TestRuntimeHTTPRealNonAgentCredentialCannotExecute(t *testing.T) {
	f, p := receiverProbeFixture(t)
	credential := uuid.NewString()
	mux := http.NewServeMux()
	api := humago.New(mux, huma.DefaultConfig("Runtime ingress test", "0.0.0"))
	api.UseMiddleware(auth.NewHumaMiddleware(api, &auth.AuthService{}, nil, nil, runtimeNonAgentResolver{credential}, &config.Config{AgentMode: true, AgentToken: receiverTestToken}))
	op := &huma.Operation{OperationID: "execute-runtime-run", Method: http.MethodPost, Path: "/api/environments/0/runtime-runs", Security: []map[string][]string{{"ApiKeyAuth": {}}}}
	api.Adapter().Handle(op, api.Middlewares().Handler(func(ctx huma.Context) { runtimeReceiverHTTPInternal(f.receiver, ctx) }))
	server := httptest.NewServer(RuntimeReceiverIngress(mux, time.Minute))
	defer server.Close()
	e := receiverEnvelope(t, f.command())
	body, err := json.Marshal(e, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	status, response := receiverHTTPCall(t, server, http.MethodPost, op.Path, credential, body)
	require.Equal(t, http.StatusForbidden, status, string(response))
	receiverCounts(t, f, 0, 0, 0)
	require.Zero(t, p.calls.Load())
	require.NotContains(t, string(response), e.Identity.RunID)
}
func TestRuntimeHTTPDisconnectDuringResolverCannotAdmit(t *testing.T) {
	f, p := receiverProbeFixture(t)
	entered, cancelled, release, handlerDone := make(chan struct{}), make(chan struct{}), make(chan struct{}), make(chan struct{})
	f.resolver.resolve = func(ctx context.Context, target RuntimeTarget) (RuntimeResolvedTarget, error) {
		close(entered)
		<-ctx.Done()
		close(cancelled)
		<-release
		return RuntimeResolvedTarget{Target: target}, nil
	}
	handler, err := NewRuntimeReceiverHTTP(f.receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: receiverTestToken}, time.Minute)
	require.NoError(t, err)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { defer close(handlerDone); handler.ServeHTTP(w, r) }))
	defer server.Close()
	e := receiverEnvelope(t, f.command())
	body, err := json.Marshal(e, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/environments/0/runtime-runs", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-Arcane-Agent-Token", receiverTestToken)
	clientDone := make(chan struct{})
	go func() {
		defer close(clientDone)
		resp, _ := server.Client().Do(req)
		if resp != nil {
			_ = resp.Body.Close()
		}
	}()
	b1Await(t, entered)
	cancel()
	b1Await(t, cancelled)
	close(release)
	b1Await(t, handlerDone)
	b1Await(t, clientDone)
	receiverCounts(t, f, 0, 0, 0)
	require.Zero(t, p.calls.Load())
}
func TestRuntimeHTTPSuccessfulCommitWinsNetworkDisconnect(t *testing.T) {
	f, p := receiverProbeFixture(t)
	server := receiverHTTPServer(t, f, time.Minute)
	pool, err := f.store.svc.db.DB.DB()
	require.NoError(t, err)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	var once sync.Once
	wrapper := &receiverCommitPool{DB: pool, after: func(*sql.Tx) { once.Do(cancel) }}
	session := f.store.svc.db.Session(&gorm.Session{NewDB: true})
	session.Config.ConnPool = wrapper
	session.Statement.ConnPool = wrapper
	f.store.svc.db = &database.DB{DB: session}
	e := receiverEnvelope(t, f.command())
	body, err := json.Marshal(e, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, server.URL+"/api/environments/0/runtime-runs", bytes.NewReader(body))
	require.NoError(t, err)
	req.Header.Set("X-Arcane-Agent-Token", receiverTestToken)
	resp, _ := server.Client().Do(req)
	if resp != nil {
		_ = resp.Body.Close()
	}
	b1Await(t, p.entered)
	answer, err := f.receiver.Query(t.Context(), e.Identity)
	require.NoError(t, err)
	require.Equal(t, e.Identity, answer.Identity)
	receiverExecute(t, f, e)
	require.EqualValues(t, 1, p.calls.Load())
	receiverCounts(t, f, 1, 1, 1)
	p.unblock()
	receiverWait(t, f, e)
}
func TestRuntimeReceiverNonqueuedIgnoredActivityInsert(t *testing.T) {
	f, p := receiverProbeFixture(t)
	command := f.command()
	command.Operation = RuntimeDownProject
	before := runtimeBoundarySnapshot(t, f.store)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER running_activity_ignore BEFORE INSERT ON activities BEGIN SELECT RAISE(IGNORE); END").Error)
	result, err := f.receiver.Execute(t.Context(), receiverEnvelope(t, command))
	var conflict *runtimeConflictInternal
	require.ErrorAs(t, err, &conflict)
	receiverZero(t, result)
	require.Equal(t, before, runtimeBoundarySnapshot(t, f.store))
	require.Zero(t, p.calls.Load())
	f.service.activity.runningMu.Lock()
	owners := len(f.service.owners)
	f.service.activity.runningMu.Unlock()
	require.Zero(t, owners)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER running_activity_ignore").Error)
}

func TestRuntimeReceiverMetadataCannotInventContinuity(t *testing.T) {
	f, p := receiverProbeFixture(t)
	metadata, err := f.receiver.ExecutorIdentity(t.Context())
	require.NoError(t, err)
	require.Equal(t, f.receiver.store.ExecutorGeneration, metadata.ExecutorGeneration)
	require.NoError(t, f.store.svc.db.Model(&RuntimeStoreGeneration{}).Where("id=1").UpdateColumn("generation", uuid.NewString()).Error)
	metadata, err = f.receiver.ExecutorIdentity(t.Context())
	require.Error(t, err)
	require.Empty(t, metadata.ExecutorGeneration)
	receiverCounts(t, f, 0, 0, 0)
	require.Zero(t, p.calls.Load())
}
