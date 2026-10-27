// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Runtime reason rejection, polling freshness and durable host permission refusals.

package runs

import (
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestRunPersistenceRejectsUndeclaredReasonEvidence(t *testing.T) {
	q := newResolutionQueueInternal(t)
	for _, field := range []string{"outcome", "target", "attempt", "attempt-target", "remote", "remote-target", "receipt"} {
		t.Run(field, func(t *testing.T) {
			run, err := q.Submit(t.Context(), st.Request{JobID: "auto-update", Trigger: "manual"})
			require.NoError(t, err)
			corrupt := func(r *st.Run) {
				switch field {
				case "outcome", "receipt":
					r.Outcome.Code = "FORBIDDEN"
				case "target":
					r.Outcome.Targets = []st.TargetOutcome{{ID: "stack", Code: "BOGUS"}}
				case "attempt":
					r.Attempts = []st.Attempt{{Outcome: st.Outcome{Code: "BOGUS"}}}
				case "attempt-target":
					r.Attempts = []st.Attempt{{Outcome: st.Outcome{Targets: []st.TargetOutcome{{ID: "stack", Code: "BOGUS"}}}}}
				case "remote":
					r.RemoteOutcome = &st.Outcome{Code: "BOGUS"}
				case "remote-target":
					r.RemoteOutcome = &st.Outcome{Targets: []st.TargetOutcome{{ID: "stack", Code: "BOGUS"}}}
				}
			}
			if field == "receipt" {
				err = q.mutateInternal(t.Context(), run.EnvironmentID, run.JobID, func(record *st.QueueRecord) error {
					r := run
					corrupt(&r)
					record.Receipts = map[string]st.Run{"receipt": r}
					return nil
				})
			} else {
				err = q.UpdateRun(t.Context(), run, func(r *st.Run) error { corrupt(r); return nil })
			}
			require.ErrorContains(t, err, "undeclared reason code")
			stored, e := q.Get(t.Context(), run.EnvironmentID, run.JobID, run.ID)
			require.NoError(t, e)
			require.Equal(t, run, stored)
			records, e := q.Records(t.Context())
			require.NoError(t, e)
			for _, record := range records {
				require.Empty(t, record.Receipts)
			}
			require.NoError(t, q.UpdateRun(t.Context(), run, func(r *st.Run) error { r.Outcome.Code = "update.permission-denied"; return nil }))
		})
	}
}
