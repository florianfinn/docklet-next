// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Bounded remote runtime implementation, active in production since #125.

package activity

import (
	"bytes"
	"context"
	"encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/auth"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
)

var receiverTestToken = uuid.NewString()

func receiverHTTPServer(t *testing.T, f *receiverFixture, budget time.Duration) *httptest.Server {
	t.Helper()
	h, err := NewRuntimeReceiverHTTP(f.receiver, &auth.AuthService{}, &config.Config{AgentMode: true, AgentToken: receiverTestToken}, budget)
	require.NoError(t, err)
	server := httptest.NewServer(h)
	t.Cleanup(server.Close)
	return server
}
func receiverHTTPCall(t *testing.T, server *httptest.Server, method, path, token string, body []byte) (int, []byte) {
	t.Helper()
	req, err := http.NewRequestWithContext(t.Context(), method, server.URL+path, bytes.NewReader(body))
	require.NoError(t, err)
	if token != "" {
		req.Header.Set("X-Arcane-Agent-Token", token)
	}
	resp, err := server.Client().Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, b
}
func TestRuntimeHTTPAuthenticationShapeAndAuthority(t *testing.T) {
	f, p := receiverProbeFixture(t)
	server := receiverHTTPServer(t, f, time.Minute)
	e := receiverEnvelope(t, f.command())
	body, err := json.Marshal(e, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	var decoded activitytypes.RuntimeExecutionEnvelope
	require.NoError(t, activitytypes.DecodeRuntimeJSON(body, &decoded), string(body))
	for _, token := range []string{"", "invalid-token"} {
		status, b := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", token, body)
		// The shared AgentCredential route rejects absent machine authority itself.
		require.Equal(t, 403, status)
		require.NotContains(t, string(b), e.Identity.RunID)
	}
	for _, bad := range []string{`null`, string(body) + ` {}`, strings.Replace(string(body), `"operation":`, `"extra":true,"operation":`, 1), strings.Replace(string(body), `"username":"Operator"`, `"username":null`, 1), strings.Replace(string(body), `"options":{`, `"options":{"forged":true,`, 1)} {
		status, _ := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, []byte(bad))
		require.Equal(t, 400, status, bad)
	}
	status, _ := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, []byte(strings.Repeat(" ", activitytypes.RuntimeMaxBodyBytes+1)))
	require.Equal(t, 413, status)
	receiverCounts(t, f, 0, 0, 0)
	require.Zero(t, p.calls.Load())
	status, b := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, body)
	require.Equal(t, 200, status, string(b))
	b1Await(t, p.entered)
	var outcome activitytypes.RuntimeExecutionOutcome
	require.NoError(t, activitytypes.DecodeRuntimeJSON(b, &outcome))
	require.NoError(t, activitytypes.ValidateRuntimeOutcome(outcome, e.Identity))
	status, b = receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, body)
	require.Equal(t, 200, status, string(b))
	require.EqualValues(t, 1, p.calls.Load())
	other := e
	other.Identity.RunID = "11111111-1111-4111-8111-111111111111"
	otherBody, err := json.Marshal(other, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	status, b = receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, otherBody)
	require.Equal(t, 409, status)
	require.JSONEq(t, `{"code":"run.busy"}`, string(b))
	require.NotContains(t, string(b), e.Identity.RunID)
	mismatch := e
	mismatch.Identity.ManagerID = "example-forged-manager"
	otherBody, err = json.Marshal(mismatch, json.FormatNilSliceAsNull(true))
	require.NoError(t, err)
	status, b = receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs", receiverTestToken, otherBody)
	require.Equal(t, 409, status)
	var refusal activitytypes.RuntimeExecutionRefusal
	require.Error(t, activitytypes.DecodeRuntimeJSON(b, &refusal))
	p.unblock()
	receiverWait(t, f, e)
}
func TestRuntimeHTTPAbsentCancelIsUncertainQueryFenceFaultIs503(t *testing.T) {
	f, p := receiverProbeFixture(t)
	server := receiverHTTPServer(t, f, time.Minute)
	e := receiverEnvelope(t, f.command())
	body, err := json.Marshal(activitytypes.RuntimeCancellation{Identity: e.Identity, Attribution: e.Attribution})
	require.NoError(t, err)
	status, b := receiverHTTPCall(t, server, "POST", "/api/environments/0/runtime-runs/"+e.Identity.RunID+"/cancel", receiverTestToken, body)
	require.Equal(t, 503, status, string(b))
	receiverCounts(t, f, 0, 0, 0)
	require.NoError(t, f.store.svc.db.Exec("CREATE TRIGGER fence_ignore BEFORE INSERT ON runtime_receipts BEGIN SELECT RAISE(IGNORE); END").Error)
	status, b = receiverHTTPCall(t, server, "GET", "/api/environments/0/runtime-runs/"+e.Identity.RunID+"?"+activitytypes.RuntimeIdentityQuery(e.Identity), receiverTestToken, nil)
	require.Equal(t, 503, status, string(b))
	receiverCounts(t, f, 0, 0, 0)
	require.NoError(t, f.store.svc.db.Exec("DROP TRIGGER fence_ignore").Error)
	receiverExecute(t, f, e)
	b1Await(t, p.entered)
	p.unblock()
	receiverWait(t, f, e)
}
func TestRuntimeHTTPExpiredIngressJoinsSlowNetworkBody(t *testing.T) {
	f, p := receiverProbeFixture(t)
	server := receiverHTTPServer(t, f, 30*time.Millisecond)
	reader, writer := io.Pipe()
	defer writer.Close()
	done := make(chan struct{})
	var status int
	var requestErr error
	ctx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, "POST", server.URL+"/api/environments/0/runtime-runs", reader)
	require.NoError(t, err)
	req.Header.Set("X-Arcane-Agent-Token", receiverTestToken)
	go func() {
		defer close(done)
		resp, err := server.Client().Do(req)
		requestErr = err
		if err == nil {
			status = resp.StatusCode
			_ = resp.Body.Close()
		}
	}()
	_, err = writer.Write([]byte(`{"identity":`))
	require.NoError(t, err)
	select {
	case <-done:
	case <-ctx.Done():
		t.Fatal("actual slow body handler did not join")
	}
	if requestErr == nil {
		require.Contains(t, []int{400, 408}, status)
	}
	receiverCounts(t, f, 0, 0, 0)
	require.Zero(t, p.calls.Load())
}
