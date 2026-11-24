// Source: docklet-next/issues/599
// Revision: 22319d6f16ef401b1c247c7a4eae599676564095
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M38 proof that a passing setup test clears an earlier host-key refusal.

package tailnet

import (
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/tailnet"
	"github.com/stretchr/testify/require"
)

// #599: adding a host was refused because the policy did not yet let tag:docklet-hub own tag:docklet-agent.
// After the console fix, the setup test mints the same agent key; its pass is the provisioning state, so the
// route works again without saving the credential again. A refusal from the test is recorded the same way.
func TestSetupTestRecordsProvisioningBothWays(t *testing.T) {
	var refuse atomic.Bool
	refuse.Store(true)
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			setupToken(w)
			return
		}
		if r.Method == "DELETE" {
			return
		}
		if refuse.Load() {
			w.WriteHeader(400)
			_ = json.NewEncoder(w).Encode(map[string]string{"message": "requested tags [" + AgentTag + "] are invalid or not permitted"})
			return
		}
		_, _ = io.WriteString(w, `{"id":"example-id","key":"`+"tskey-"+"auth-example-recovered"+`"}`)
	})
	_, _, err := m.ProvisionHostKey(t.Context(), "")
	var problem *dto.Problem
	require.ErrorAs(t, err, &problem)
	require.Equal(t, dto.CodeTagsNotPermitted, problem.Code)
	require.Equal(t, "rejected", m.Status().Provisioning.State)

	refuse.Store(false)
	status, body := setupHTTP(t, setupRouter(m, true, false), t.Context())
	require.Equal(t, 200, status, string(body))
	for _, check := range setupResult(t, body).Checks {
		require.Equal(t, "pass", check.State, check.ID)
	}
	require.Equal(t, "ok", m.Status().Provisioning.State)
	require.NoError(t, m.RouteRefusal())
	key, _, err := m.ProvisionHostKey(t.Context(), "")
	require.NoError(t, err)
	require.True(t, strings.HasPrefix(key, "tskey-"))

	refuse.Store(true)
	status, body = setupHTTP(t, setupRouter(m, true, false), t.Context())
	require.Equal(t, 200, status, string(body))
	require.Equal(t, dto.CodeTagsNotPermitted, setupResult(t, body).Checks[1].Code)
	require.Equal(t, "rejected", m.Status().Provisioning.State)
	require.Equal(t, []string{AgentTag}, m.Status().Provisioning.RefusedTags)
}

// An identical credential save while the setup test waits for its token fences the test: the save's own check
// decides provisioning, and the older test's refusal never overwrites it.
func TestSetupTestFencedBySaveDuringToken(t *testing.T) {
	entered, release := make(chan struct{}), make(chan struct{})
	var tokens atomic.Int32
	m := setupManager(t, func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/token") {
			if tokens.Add(1) == 1 {
				close(entered)
				<-release
			}
			setupToken(w)
			return
		}
		if r.Method == "DELETE" {
			return
		}
		w.WriteHeader(400)
		_ = json.NewEncoder(w).Encode(map[string]string{"message": "requested tags [" + AgentTag + "] are invalid or not permitted"})
	})
	m.run = &run{ctx: t.Context()}
	done := make(chan []byte, 1)
	go func() { _, body := setupHTTP(t, setupRouter(m, true, false), t.Context()); done <- body }()
	<-entered
	require.True(t, m.updateCredential(true, m.cred))
	require.Eventually(t, func() bool { return m.Status().Provisioning.State == "ok" }, 5*time.Second, 5*time.Millisecond)
	close(release)
	<-done
	require.Equal(t, "ok", m.Status().Provisioning.State)
}
