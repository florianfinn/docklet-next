// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package project

import (
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestRestorePointProjectionRetentionParity(t *testing.T) {
	for _, failure := range []string{"registration", "snapshot"} {
		t.Run(failure, func(t *testing.T) {
			f := newLifecycleFixture(t)
			snapshot := f.establishBaseline()
			store := f.service.lifecycleStore()
			point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
			require.NoError(t, err)
			require.NoError(t, store.Available(t.Context(), point.ID))
			if failure == "registration" {
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", point.ID).Update("registration_id", "old-registration").Error)
			} else {
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", point.ID).Update("payload", "synthetic-invalid-snapshot").Error)
			}
			rows, err := f.service.ListImageRestorePoints(t.Context(), f.project.ID, true)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			require.False(t, rows[0].Available)
			require.NotNil(t, rows[0].ProtectionReason)
			require.Equal(t, dto.HealthyBaseline, *rows[0].ProtectionReason)
			require.Equal(t, dto.HoldActive, rows[0].HoldState)
			var stored []life.Point
			require.NoError(t, f.db.Where("available = ?", true).Find(&stored).Error)
			require.Equal(t, life.PlanRetention(stored, nil).Reasons[point.ID], *rows[0].ProtectionReason)
			require.Empty(t, life.RetentionDeletions(stored, nil))
		})
	}
}

// The executor fake begins effects only after the production Prepare succeeds.
// This tests the final safety authority independently of the advisory hub read.
func TestPointDeleteOrders(t *testing.T) {
	for _, order := range []string{"delete-first", "admitted-not-dispatched", "late-delete", "running", "unknown-delete"} {
		t.Run(order, func(t *testing.T) {
			f := newLifecycleFixture(t)
			snapshot := f.establishBaseline()
			store := f.service.lifecycleStore()
			point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
			require.NoError(t, err)
			require.NoError(t, store.Available(t.Context(), point.ID))
			require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
			effects := 0
			selectPoint := func() error {
				_, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), point.ID, "settled")
				if err == nil {
					effects++
				}
				return err
			}
			switch order {
			case "delete-first", "late-delete", "unknown-delete":
				require.NoError(t, store.Delete(t.Context(), point.ID))
				err = selectPoint()
				var problem *imageDTO.OperationError
				require.ErrorAs(t, err, &problem)
				require.Equal(t, "restore.unavailable", problem.Problem.Code)
				require.Equal(t, 409, problem.Status)
				require.Zero(t, effects)
				var n int64
				require.NoError(t, f.db.Model(&life.Run{}).Where("selected_point_id = ?", point.ID).Count(&n).Error)
				require.Zero(t, n)
			case "running":
				require.NoError(t, selectPoint())
				require.Equal(t, 1, effects)
				err = store.Delete(t.Context(), point.ID)
				var problem *imageDTO.OperationError
				require.ErrorAs(t, err, &problem)
				require.Equal(t, "restore.in-use", problem.Problem.Code)
			case "admitted-not-dispatched":
				// This fixture represents the committed receipt/intent before executor selection.
				// Unsettled hub intent/receipt evidence is covered by the activity and
				// transport tests; local Store.Held also preserves that committed evidence.
				now := time.Now().UTC()
				receipt := activity.RuntimeReceipt{RunID: uuid.NewString(), Kind: "receipt", StoreGeneration: f.binding.StoreGeneration, ManagerID: f.binding.ManagerID, RegistrationID: f.binding.RegistrationID, EnvironmentID: "0", ExecutorGeneration: f.binding.ExecutorGeneration, RequestDigest: "synthetic-digest", CallerID: new("operator"), OperationID: new("restore-project"), TargetKind: new("stack"), TargetID: &f.project.ID, GateScope: new("stack:" + f.project.ID), GateKey: new("restore"), AdmittedAt: &now, DispatchState: new("pending"), ExecutorState: new("pending"), FinalWriteAt: now}
				require.NoError(t, f.db.Create(&receipt).Error)
				payload, e := json.Marshal(dto.LifecycleIntent{Request: dto.LifecycleRequest{PointID: point.ID}})
				require.NoError(t, e)
				require.NoError(t, f.db.Create(&activity.RuntimeImageIntent{RunID: receipt.RunID, Payload: string(payload)}).Error)
				err = store.Delete(t.Context(), point.ID)
				var denied *imageDTO.OperationError
				require.ErrorAs(t, err, &denied)
				require.Equal(t, "restore.in-use", denied.Problem.Code)
				require.Zero(t, effects)
			}
		})
	}
}

func TestPointPrepareUntypedPersistenceFailure(t *testing.T) {
	f := newLifecycleFixture(t)
	snapshot := f.establishBaseline()
	require.NoError(t, f.db.Exec("CREATE TRIGGER reject_point BEFORE INSERT ON restore_points BEGIN SELECT RAISE(ABORT, 'synthetic persistence failure'); END").Error)
	_, err := f.service.lifecycleStore().Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
	var problem *imageDTO.OperationError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "update.persistence-failed", problem.Problem.Code)
	require.Equal(t, 500, problem.Status)
}
