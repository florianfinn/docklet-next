// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original complete received-outcome versus deadline decision table.

package environment

import (
	"errors"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"testing"
	"time"
)

func TestRestartDeadlineReceivedOutcomeMatrix(t *testing.T) {
	for _, kind := range []string{"late-202", "late-409", "early-ack-late-write", "early-refusal-late-write", "late-proof-ack", "late-proof-no-ack"} {
		t.Run(kind, func(t *testing.T) {
			f := newRestartFixture(t)
			w := f.reserve(t)
			switch kind {
			case "late-202", "late-409":
				if kind == "late-409" {
					f.sendStatus = 409
				}
				f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
					response, err := f.roundTrip(r)
					if r.URL.Path == "/api/system/agent/restart" {
						f.now = w.row.Deadline
					}
					return response, err
				})
				if !f.step(t, w) {
					require.True(t, f.step(t, w))
				}
				assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
				require.Nil(t, w.row.AcknowledgedAt)
			case "early-ack-late-write", "early-refusal-late-write":
				if kind == "early-refusal-late-write" {
					f.sendStatus = 409
				}
				fail := true
				require.NoError(t, f.c.service.db.Callback().Update().Before("gorm:update").Register("restart-deadline-write", func(tx *gorm.DB) {
					if tx.Statement.Table == "agent_restart_requests" && f.calls > 0 && fail {
						tx.AddError(errors.New("synthetic joined-outcome write failure"))
					}
				}))
				done, err := f.c.advance(t.Context(), w)
				require.Error(t, err)
				require.False(t, done)
				require.NotNil(t, w.pending)
				fail = false
				f.now = w.row.Deadline.Add(time.Second)
				if !f.step(t, w) {
					require.True(t, f.step(t, w))
				}
				if kind == "early-ack-late-write" {
					assertRestartCode(t, w, "failed", "agent.restart-timeout")
					require.NotNil(t, w.row.AcknowledgedAt)
				} else {
					assertRestartCode(t, w, "failed", "agent.restart-unsupported")
				}
				require.NoError(t, f.c.service.db.Callback().Update().Remove("restart-deadline-write"))
			default:
				f.sendError = kind == "late-proof-no-ack"
				require.False(t, f.step(t, w))
				f.instance = "example-new"
				f.now = w.row.Deadline.Add(-time.Second)
				reads := 0
				f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
					response, err := f.roundTrip(r)
					if r.URL.Path == "/api/health" {
						reads++
						if reads == 2 {
							f.now = w.row.Deadline
						}
					}
					return response, err
				})
				require.True(t, f.step(t, w))
				if kind == "late-proof-ack" {
					assertRestartCode(t, w, "failed", "agent.restart-timeout")
				} else {
					assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
				}
			}
			require.Equal(t, 1, f.calls)
		})
	}
}
func TestRestartSendContextBoundedByStoredDeadline(t *testing.T) {
	f := newRestartFixture(t)
	w := f.reserve(t)
	f.now = w.row.Deadline.Add(-2 * time.Second)
	f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
		if r.URL.Path == "/api/system/agent/restart" {
			deadline, ok := r.Context().Deadline()
			require.True(t, ok)
			require.LessOrEqual(t, time.Until(deadline), 2*time.Second)
		}
		return f.roundTrip(r)
	})
	require.False(t, f.step(t, w))
	require.Equal(t, 1, f.calls)
}
