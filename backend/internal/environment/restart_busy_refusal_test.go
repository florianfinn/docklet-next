// Source: docklet-next/issues/368
// Revision: 32e24a1b5260c98b6ce490ce2c1f6f50413c1e83
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original deadline decision table for the agent's busy refusal.

package environment

import (
	"net/http"
	"testing"

	"github.com/stretchr/testify/require"
)

// A busy refusal is definitive like agent.restart-unsupported: before the
// deadline the run fails with host.busy, an answer at the deadline stays
// unconfirmed under the existing rule.
func TestRestartBusyRefusalFollowsTheDeadlineRule(t *testing.T) {
	for _, kind := range []string{"timely", "late"} {
		t.Run(kind, func(t *testing.T) {
			f := newRestartFixture(t)
			f.sendStatus, f.sendCode = 409, "host.busy"
			w := f.reserve(t)
			if kind == "late" {
				f.c.service.httpClient.Transport = agentRoundTrip(func(r *http.Request) (*http.Response, error) {
					response, err := f.roundTrip(r)
					if r.URL.Path == "/api/system/agent/restart" {
						f.now = w.row.Deadline
					}
					return response, err
				})
			}
			if !f.step(t, w) {
				require.True(t, f.step(t, w))
			}
			if kind == "timely" {
				assertRestartCode(t, w, "failed", "host.busy")
			} else {
				assertRestartCode(t, w, "unknown", "agent.restart-unconfirmed")
			}
			require.Nil(t, w.row.AcknowledgedAt)
			require.Equal(t, 1, f.calls)
		})
	}
}
