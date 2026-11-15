// Source: docklet-next/issues/441
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proof of the bounded failed-candidate store.

package policies

import (
	"fmt"
	"strings"
	"testing"
	"time"

	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

// R1S1-3: the store keeps the newest MaxSuppressedCandidates tuples per policy and
// target, so a later binding always fits its bound.
func TestFailedCandidatesKeepNewestWithinBindingBound(t *testing.T) {
	f := newStoreFixture(t)
	p := f.policy(t, "scheduled")
	start := time.Now().UTC()
	for run := range 3 {
		child, _, err := f.PrepareChild(t.Context(), p, fmt.Sprintf("window-%d", run))
		require.NoError(t, err)
		require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return AdmitChild(tx, &dto.PolicyBinding{ChildID: child.ID}, child.ID) }))
		child.Status, child.RuntimeID = "admitted", child.ID
		var applied []dto.AppliedCandidate
		for i := range 30 {
			applied = append(applied, dto.AppliedCandidate{Service: fmt.Sprintf("svc-%02d", i), Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: fmt.Sprintf("sha256:%s%02d", strings.Repeat("a", 62), run)})
		}
		checked := CheckedChildResult{TargetIdentity: p.TargetIdentity, Code: "update.auto-restored", Recovery: &dto.RecoveryResult{Outcome: "restored"}, Applied: applied, SettledAt: start.Add(time.Duration(run) * time.Hour)}
		require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error { return SettleChildTx(tx, child, checked) }))
	}
	rows, err := f.FailedCandidates(t.Context(), p.ID, p.TargetIdentity)
	require.NoError(t, err)
	require.Len(t, rows, dto.MaxSuppressedCandidates)
	runs := map[string]int{}
	for _, row := range rows {
		runs[row.Manifest[len(row.Manifest)-2:]]++
	}
	require.Equal(t, map[string]int{"02": 30, "01": 30, "00": 4}, runs, "newest runs are kept first")
}
