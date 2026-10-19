// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Actual receipt and composed-settlement projection cohorts.

package activity

import (
	"context"
	"encoding/json"
	"strings"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func b3Projection(t *testing.T, f *b1FixtureInternal, id string) activitytypes.RuntimeSnapshot {
	t.Helper()
	var snapshot activitytypes.RuntimeSnapshot
	b1Tx(t, f, func(tx *gorm.DB, _ time.Time) error {
		row, err := txRowB3(tx, id)
		if err != nil {
			return err
		}
		snapshot, err = runtimeSnapshotTxInternal(tx, row)
		return err
	})
	return snapshot
}
func txRowB3(tx *gorm.DB, id string) (*RuntimeReceipt, error) {
	var row RuntimeReceipt
	err := tx.First(&row, "run_id = ?", id).Error
	return &row, err
}

func TestRuntimeProjectionReceiptCohorts(t *testing.T) {
	for _, cohort := range []struct {
		name, state, code string
		wantStatus        activitytypes.Status
		wantKnowledge     activitytypes.RuntimeOutcomeState
		cancel            bool
	}{
		{"admitted-pending", "pending", "run.started", activitytypes.StatusRunning, activitytypes.RuntimeOutcomeUnknown, false},
		{"accepted", "accepted", "run.started", activitytypes.StatusRunning, activitytypes.RuntimeOutcomeKnown, false},
		{"executing", "executing", "run.started", activitytypes.StatusRunning, activitytypes.RuntimeOutcomeKnown, false},
		{"terminal-success", "success", "activity.success", activitytypes.StatusSuccess, activitytypes.RuntimeOutcomeKnown, false},
		{"terminal-failed", "failed", "runtime.failed", activitytypes.StatusFailed, activitytypes.RuntimeOutcomeKnown, false},
		{"terminal-cancelled", "cancelled", "runtime.cancelled", activitytypes.StatusCancelled, activitytypes.RuntimeOutcomeKnown, true},
		{"local-interrupted", "interrupted", "runtime.interrupted", activitytypes.StatusFailed, activitytypes.RuntimeOutcomeKnown, false},
		{"remote-dispatch-unknown", "unknown", "run.started", activitytypes.StatusRunning, activitytypes.RuntimeOutcomeUnknown, true},
		{"admin-released-unknown", "released", "runtime.interrupted", activitytypes.StatusFailed, activitytypes.RuntimeOutcomeUnknown, false},
	} {
		t.Run(cohort.name, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			if strings.Contains(cohort.name, "remote") || cohort.state == "released" {
				c.Store.EnvironmentID = "example-remote"
				c.Store.ExecutorGeneration = uuid.NewString()
			}
			id := b3Admit(t, f, c)
			if cohort.cancel && cohort.state != "unknown" {
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					_, err := runtimeCancelIntentTxInternal(tx, now, b1Identity(t, c, id))
					return err
				})
			}
			switch cohort.state {
			case "accepted", "executing":
				b3Phase(t, f, c, id, cohort.state)
			case "success", "failed", "cancelled":
				if cohort.state != "cancelled" {
					b3Phase(t, f, c, id, "executing")
				}
				code := activitytypes.Code(cohort.code)
				message := "Runtime workflow completed"
				if cohort.state != "success" {
					message = "Runtime workflow or output failed; effects unverified"
				}
				ctx := context.WithValue(t.Context(), gateRunContextKey{}, id)
				f.store.svc.runningMu.Lock()
				err := f.service.transactionInternal(ctx, func(tx *gorm.DB, now time.Time) error {
					row, err := runtimeReadTxInternal(tx, b1Identity(t, c, id))
					if err != nil {
						return err
					}
					_, _, err = runtimeCompleteTxInternal(tx, now, b1Identity(t, c, id), runtimeLocalResultInternal{
						state: cohort.state, code: code, message: message, startedAt: row.ExecutorStartedAt, settledAt: now})
					return err
				})
				f.store.svc.runningMu.Unlock()
				require.NoError(t, err)
			case "interrupted":
				count, err := f.service.SettleLocalStartup(t.Context(), c.Store)
				require.NoError(t, err)
				require.EqualValues(t, 1, count)
			case "unknown":
				// Storage projection only: F-c owns actual remote transport/reconciliation.
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					identity := b1Identity(t, c, id)
					_, err := runtimeDispatchTxInternal(tx, now, identity, false)
					if err != nil {
						return err
					}
					_, err = runtimeCancelIntentTxInternal(tx, now, identity)
					if err != nil {
						return err
					}
					_, err = runtimeEvidenceTxInternal(tx, now, identity, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
					return err
				})
			case "released":
				// Storage projection only: this is not an administrator release workflow.
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					identity := b1Identity(t, c, id)
					_, err := runtimeEvidenceTxInternal(tx, now, identity, runtimeEvidenceInternal{State: "unknown", UnknownReason: new("executor-unavailable")})
					if err != nil {
						return err
					}
					_, err = runtimeDispositionTxInternal(tx, now, identity, runtimeDispositionInternal{
						Disposition: "released", By: "example-admin", Reason: "Stopped and inspected", ReleaseAcknowledged: true})
					return err
				})
			}
			snapshot := b3Projection(t, f, id)
			require.Equal(t, id, snapshot.ID)
			require.Equal(t, cohort.wantStatus, snapshot.Status)
			require.Equal(t, cohort.wantKnowledge, snapshot.OutcomeState)
			require.Equal(t, cohort.cancel, snapshot.CancelRequested)
			require.Equal(t, cohort.code, snapshot.MessageCode)
			if cohort.state == "released" {
				before := b3Receipt(t, f, id)
				b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
					_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, c, id), runtimeEvidenceInternal{
						State: "success", SettledAt: new(now), ResultCode: new("activity.success")})
					return err
				})
				late := b3Projection(t, f, id)
				require.Equal(t, activitytypes.StatusFailed, late.Status)
				require.Equal(t, string(activitytypes.CodeRuntimeInterrupted), late.MessageCode)
				require.Equal(t, activitytypes.RuntimeOutcomeKnown, late.OutcomeState)
				after := b3Receipt(t, f, id)
				require.Equal(t, before.HubDisposition, after.HubDisposition)
				require.Equal(t, before.HubDispositionReason, after.HubDispositionReason)
			}
			if cohort.state == "interrupted" || cohort.state == "success" {
				_, err := f.store.svc.DeleteHistory(t.Context(), c.Store.EnvironmentID)
				require.NoError(t, err)
				compact := b3Projection(t, f, id)
				require.Equal(t, snapshot.ID, compact.ID)
				require.Equal(t, snapshot.Status, compact.Status)
				require.Equal(t, snapshot.MessageCode, compact.MessageCode)
				require.Equal(t, snapshot.OutcomeState, compact.OutcomeState)
				require.Empty(t, compact.Progress)
			}
		})
	}
}

func TestRuntimeProjectionLegacyActivityHasNoInventedKnowledge(t *testing.T) {
	f := b1Fixture(t)
	id := uuid.NewString()
	require.NoError(t, f.store.svc.db.Create(&Activity{ID: id, EnvironmentID: "0", Type: activitytypes.TypeResourceAction,
		Status: activitytypes.StatusRunning, StartedAt: f.store.now}).Error)
	row := b3Activity(t, f, id)
	data, err := json.Marshal(activityToDTOInternal(&row))
	require.NoError(t, err)
	require.NotContains(t, string(data), "outcomeState")
	require.NotContains(t, string(data), "cancelRequested")
}

func TestRuntimeProjectionQueuedRequiresLocalEligiblePendingEvidence(t *testing.T) {
	for _, tc := range []struct {
		name, state     string
		operation       RuntimeOperation
		remote, missing bool
		want            activitytypes.Status
	}{
		{name: "pending", state: "pending", operation: RuntimeDeployProject, want: activitytypes.StatusQueued},
		{name: "accepted-redeploy", state: "accepted", operation: RuntimeRedeployProject, want: activitytypes.StatusQueued},
		{name: "executing-stale-queued", state: "executing", operation: RuntimeDeployProject, want: activitytypes.StatusRunning},
		{name: "success-stale-queued", state: "success", operation: RuntimeDeployProject, want: activitytypes.StatusSuccess},
		{name: "failed-stale-queued", state: "failed", operation: RuntimeDeployProject, want: activitytypes.StatusFailed},
		{name: "cancelled-stale-queued", state: "cancelled", operation: RuntimeDeployProject, want: activitytypes.StatusCancelled},
		{name: "unknown-stale-queued", state: "unknown", operation: RuntimeDeployProject, want: activitytypes.StatusRunning},
		{name: "remote-pending", state: "pending", operation: RuntimeDeployProject, remote: true, want: activitytypes.StatusRunning},
		{name: "remote-accepted", state: "accepted", operation: RuntimeRedeployProject, remote: true, want: activitytypes.StatusRunning},
		{name: "unqueued-operation", state: "pending", operation: RuntimeDownProject, want: activitytypes.StatusRunning},
		{name: "missing-local-evidence", state: "pending", operation: RuntimeDeployProject, missing: true, want: activitytypes.StatusRunning},
	} {
		t.Run(tc.name, func(t *testing.T) {
			f := b1Fixture(t)
			c := f.command()
			c.Operation = tc.operation
			if tc.remote {
				c.Store.EnvironmentID = "example-remote"
				c.Store.ExecutorGeneration = uuid.NewString()
			}
			id := b3Admit(t, f, c)
			require.NoError(t, f.store.svc.db.Model(&Activity{}).Where("id = ?", id).UpdateColumn("status", activitytypes.StatusQueued).Error)
			b1Tx(t, f, func(tx *gorm.DB, now time.Time) error {
				if tc.state == "pending" {
					return nil
				}
				evidence := runtimeEvidenceInternal{State: tc.state}
				if tc.state == "unknown" {
					evidence.UnknownReason = new("executor-unavailable")
				}
				if tc.state == "success" || tc.state == "failed" || tc.state == "cancelled" {
					evidence.SettledAt = new(now)
				}
				_, err := runtimeEvidenceTxInternal(tx, now, b1Identity(t, c, id), evidence)
				return err
			})
			if tc.missing {
				require.NoError(t, f.store.svc.db.Where("id = ?", id).Delete(&Activity{}).Error)
			}
			receipt := b3Receipt(t, f, id)
			knowledge := runtimeKnowledgeInternal(&receipt)
			snapshot := b3Projection(t, f, id)
			require.Equal(t, tc.want, snapshot.Status)
			require.Equal(t, knowledge, snapshot.RuntimeKnowledge, "queue projection must not alter receipt knowledge")
		})
	}
}
