// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original Go-serialized lifecycle evidence for the web consumer.

package updater

import (
	"bytes"
	"encoding/json"
	a "github.com/florianfinn/docklet-next/types/activity"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"os"
	"testing"

	"github.com/florianfinn/docklet-next/types/updater"
)

// TestRecoveryWebFixture checks the exact producer JSON, including omitted fields.
// UPDATE_WEB_FIXTURES=1 deliberately regenerates the checked-in consumer fixture.
func TestRecoveryWebFixture(t *testing.T) {
	rows := []updater.LifecycleResult{}
	for _, row := range []struct{ outcome, code string }{
		{"restored", "update.unstable"}, {"restore-failed", "restore.deploy-failed"},
		{"unavailable", "restore.files-changed"}, {"interrupted", "runtime.interrupted"},
		{"not-needed", "update.snapshot-unavailable"},
	} {
		rows = append(rows, updater.LifecycleResult{RunID: "example-run", Code: "update.unstable", Recovery: &updater.RecoveryResult{Outcome: row.outcome, Code: row.code}})
	}
	targets := []st.TargetOutcome{}
	for _, result := range rows {
		target, unknown := imagePolicySnapshotTarget("example", result.RunID, a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusFailed, ImageLifecycle: &result}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}})
		if unknown {
			t.Fatal("fixture unexpectedly projects an unknown target")
		}
		targets = append(targets, qualifyUpdateTarget("direct", "stack", "project", target))
	}
	skipped, unknown := imagePolicySnapshotTarget("example", "example-run", a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusFailed, ImageLifecycle: &updater.LifecycleResult{Code: "update.candidate-failed", Recovery: &updater.RecoveryResult{Outcome: "not-needed"}}}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}})
	if unknown {
		t.Fatal("suppressed fixture unexpectedly unknown")
	}
	targets = append(targets, qualifyUpdateTarget("direct", "stack", "project", skipped))
	writeWebFixture(t, "update-target-outcomes.json", targets)
	writeWebFixture(t, "update-recovery.json", rows)
}
func writeWebFixture(t *testing.T, name string, value any) {
	t.Helper()
	body, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		t.Fatal(err)
	}
	body = append(body, '\n')
	path := "../../../web/src/data/http/testdata/" + name
	if os.Getenv("UPDATE_WEB_FIXTURES") == "1" {
		if err := os.WriteFile(path, body, 0644); err != nil {
			t.Fatal(err)
		}
	}
	expected, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(expected, body) {
		t.Fatal("web lifecycle fixture differs from the Go producer; regenerate with UPDATE_WEB_FIXTURES=1")
	}
}
