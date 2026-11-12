// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original ambiguous-send deadline and post-observation read-failure guards.

package environment

import (
	"errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestRestartAmbiguousSentCommitAtDeadlineUsesDurableState(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	env, err := f.c.bound(t.Context(), w.row)
	require.NoError(t, err)
	w.prepared = &restartPreparation{env: *env, old: f.instance}
	require.NoError(t, f.c.service.db.Model(&AgentRestartRequest{}).Where("run_id=?", w.row.RunID).Updates(map[string]any{"state": "sent", "old_instance": f.instance, "sent_at": f.now}).Error)
	f.now = w.row.Deadline
	if !f.step(t, w) {
		require.True(t, f.step(t, w))
	}
	assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
	require.Zero(t, f.calls)
}

func TestRestartRetainsProofAcrossPostObservationBindingReadFailure(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	require.False(t, f.step(t, w))
	f.instance = "example-new"
	fail := false
	healthReads := 0
	f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := f.roundTrip(r)
		if r.URL.Path == "/api/health" {
			healthReads++
			if healthReads == 2 {
				fail = true
			}
		}
		return response, err
	})
	require.NoError(t, f.c.service.db.Callback().Query().Before("gorm:query").Register("restart-after-proof-read", func(tx *gorm.DB) {
		if tx.Statement.Table == "environments" && fail {
			tx.AddError(errors.New("synthetic post-observation binding read failure"))
		}
	}))
	done, err := f.c.advance(t.Context(), w)
	require.Error(t, err)
	require.False(t, done)
	require.Equal(t, 2, healthReads)
	require.NotNil(t, w.pending, "joined proof must survive the failed post-observation read")
	require.Equal(t, "connected", w.pending.state)
	fail = false
	require.NoError(t, f.c.service.db.Callback().Query().Remove("restart-after-proof-read"))
	f.unreachable = true
	f.now = w.row.Deadline.Add(time.Minute)
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "connected", "")
	require.Equal(t, 1, f.calls)
}

type restartBrokenAcknowledgementBody struct{ reads *int }

func (b restartBrokenAcknowledgementBody) Read([]byte) (int, error) {
	*b.reads++
	return 0, io.ErrUnexpectedEOF
}
func (restartBrokenAcknowledgementBody) Close() error { return nil }

func TestRestartReceived202SurvivesBrokenResponseBody(t *testing.T) {
	f := newRestartFixture(t)
	reads := 0
	f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		response, err := f.roundTrip(r)
		if r.URL.Path == "/api/system/agent/restart" {
			response.Body = restartBrokenAcknowledgementBody{reads: &reads}
		}
		return response, err
	})
	w := f.reserve(t)
	require.False(t, f.step(t, w))
	require.NotNil(t, w.row.AcknowledgedAt, "received 202 status is acknowledgement even if its optional instance body breaks")
	require.Zero(t, reads, "the response instance is not needed for acknowledgement")
	f.now = w.row.Deadline
	require.True(t, f.step(t, w))
	assertRestartCode(t, w, "failed", "agent.restart-timeout")
	require.Equal(t, 1, f.calls)
}
