// Source: docklet-next/issues/439
// Revision: 7564d315b6fbfd50e17265f52af0e2c70c450af7
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original agent refusal settlement proof (M21 R1-S8-02).

package upgrade

import (
	"context"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	dto "github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
)

// The agent refuses after the hub's check said available (its own backup work
// makes the host busy). The hub run settles at once with the agent's code and
// the host is free again; nothing reached the Engine.
func TestAgentRefusalSettlesTheHubRunAtOnceDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			configureUpdateFeed(t, f)
			f.agent.SetBackupBusy(func(context.Context) (bool, error) { return true, nil })
			result, err := f.hub.StartUpdate(t.Context(), f.remote.ID, dto.SelfUpdateEnvelope{}, "operator", "operator")
			require.NoError(t, err)
			var op *core.Operation
			require.Eventually(t, func() bool {
				op, err = f.hub.remoteStore(f.remote.ID).Read(t.Context())
				return err == nil && op != nil && op.Phase == "settled"
			}, 5*time.Second, 50*time.Millisecond, "a definitive refusal must not wait for the deadline")
			require.Equal(t, []string{"failed-pre-effect", "host.busy"}, []string{op.Outcome, op.Code})
			var run activity.Activity
			require.NoError(t, f.db.First(&run, "id=?", result.RunID).Error)
			require.Equal(t, activityDTO.StatusFailed, run.Status)
			require.NoError(t, core.CheckHostTx(f.db.WithContext(t.Context()), f.remote.ID))
			require.EqualValues(t, 1, f.posts.Load())
			f.daemon.mu.Lock()
			defer f.daemon.mu.Unlock()
			require.Empty(t, f.daemon.mutations)
		})
	}
}

// An env file outside the project directory is reported as an unsupported
// layout by the check, before the action is offered.
func TestOutsideEnvFileIsNotEligibleDirectAndEdge(t *testing.T) {
	updateStamp(t)
	for _, edge := range []bool{false, true} {
		t.Run(map[bool]string{false: "direct", true: "edge"}[edge], func(t *testing.T) {
			f := newUpdateTransport(t, edge)
			configureUpdateFeed(t, f)
			f.daemon.mu.Lock()
			f.daemon.envFile = "/etc/example/agent.env"
			f.daemon.mu.Unlock()
			check, err := f.hub.Check(t.Context(), f.remote.ID)
			require.NoError(t, err)
			require.Equal(t, []string{"not-eligible", core.CodeLayout}, []string{check.Eligibility.State, check.Eligibility.Code})
		})
	}
}

// Only a decoded 4xx problem is a refusal; anything else may have executed.
func TestAgentRefusalClassification(t *testing.T) {
	for _, tc := range []struct {
		name    string
		err     error
		code    string
		refused bool
	}{
		{"host busy", &remenv.StatusError{StatusCode: 409, Body: []byte(`{"code":"host.busy","status":409}`)}, "host.busy", true},
		{"huma detail", &remenv.StatusError{StatusCode: 409, Body: []byte(`{"status":409,"detail":"update.requires-hub-update"}`)}, core.CodeHub, true},
		{"unknown code", &remenv.StatusError{StatusCode: 403, Body: []byte(`{"detail":"forbidden"}`)}, core.CodeFailed, true},
		{"server error", &remenv.StatusError{StatusCode: 503, Body: []byte(`{"code":"host.busy"}`)}, "", false},
		{"transport", &remenv.TransportError{Err: context.DeadlineExceeded}, "", false},
		{"success", nil, "", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, refused := agentRefusal(tc.err)
			require.Equal(t, []any{tc.code, tc.refused}, []any{code, refused})
		})
	}
}
