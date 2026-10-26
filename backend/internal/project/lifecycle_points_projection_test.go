// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Producer and transport regression proofs for #229 point projection.

package project

import (
	"context"
	"encoding/json"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
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

func TestPointPrepareSelectionGuards(t *testing.T) {
	for _, field := range []string{"available", "target_id", "target_identity", "registration_id", "missing"} {
		t.Run(field, func(t *testing.T) {
			f := newLifecycleFixture(t)
			snapshot := f.establishBaseline()
			store := f.service.lifecycleStore()
			point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
			require.NoError(t, err)
			require.NoError(t, store.Available(t.Context(), point.ID))
			selected := point.ID
			switch field {
			case "available":
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", point.ID).Update(field, false).Error)
			case "missing":
				selected = uuid.NewString()
			default:
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", point.ID).Update(field, "foreign").Error)
			}
			_, err = store.Prepare(t.Context(), snapshot, uuid.NewString(), selected, "settled")
			var problem *imageDTO.OperationError
			require.ErrorAs(t, err, &problem)
			require.Equal(t, "restore.unavailable", problem.Problem.Code)
			require.Equal(t, 409, problem.Status)
			var n int64
			require.NoError(t, f.db.Model(&life.Run{}).Where("selected_point_id = ?", selected).Count(&n).Error)
			require.Zero(t, n)
		})
	}
}

func TestPointPrepareUpdatePersistenceFailure(t *testing.T) {
	f := newLifecycleFixture(t)
	snapshot := f.establishBaseline()
	store := f.service.lifecycleStore()
	point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
	require.NoError(t, err)
	require.NoError(t, store.Available(t.Context(), point.ID))
	require.NoError(t, f.db.Exec("CREATE TRIGGER reject_selection BEFORE UPDATE ON restore_points BEGIN SELECT RAISE(ABORT, 'synthetic selection persistence failure'); END").Error)
	_, err = store.Prepare(t.Context(), snapshot, uuid.NewString(), point.ID, "settled")
	var problem *imageDTO.OperationError
	require.ErrorAs(t, err, &problem)
	require.Equal(t, "update.persistence-failed", problem.Problem.Code)
	require.Equal(t, 500, problem.Status)
}

func TestRestorePointDeferredTrimRechecksAnchors(t *testing.T) {
	for _, change := range []string{"restore-target", "healthy-baseline", "hold"} {
		t.Run(change, func(t *testing.T) {
			f := newLifecycleFixture(t)
			snapshot := f.establishBaseline()
			store := f.service.lifecycleStore()
			var points []life.Point
			for i := 0; i < 7; i++ {
				point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "partial-source")
				require.NoError(t, err)
				require.NoError(t, store.Available(t.Context(), point.ID))
				require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
			}
			require.NoError(t, f.db.Order("created_at DESC, id DESC").Find(&points).Error)
			id := points[6].ID
			require.Contains(t, life.RetentionDeletions(points, nil), id)
			committed, resume := make(chan struct{}), make(chan struct{})
			done := make(chan error, 1)
			go func() { close(committed); <-resume; done <- store.TrimPoint(t.Context(), f.project.ID, id) }()
			<-committed
			switch change {
			case "restore-target":
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", id).Update("last_restored_at", time.Now().UTC()).Error)
			case "healthy-baseline":
				require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", id).Update("source_state", "settled").Error)
			case "hold":
				require.NoError(t, f.db.Create(&life.Run{ID: uuid.NewString(), PointID: id}).Error)
			}
			close(resume)
			require.NoError(t, <-done)
			var n int64
			require.NoError(t, f.db.Model(&life.Point{}).Where("id = ?", id).Count(&n).Error)
			require.EqualValues(t, 1, n)
		})
	}
}

func TestRestorePointProjectionFiveFailedRestores(t *testing.T) {
	f := newLifecycleFixture(t)
	f.establishBaseline()
	require.Equal(t, activityDTO.StatusSuccess, f.wait(f.admit(f.intent("rebuild"))).Status)
	target := lifecyclePoints(t, f)[0].ID
	f.up = func(context.Context, *compose.Project) error { return errors.New("synthetic restore failure") }
	for attempt := 0; attempt < 5; attempt++ {
		require.Equal(t, activityDTO.StatusFailed, f.wait(f.admit(lifecycleRestore(t, f, target))).Status)
		require.NoError(t, f.service.ReconcileImageLifecycle(t.Context()))
		rows, err := f.service.ListImageRestorePoints(t.Context(), f.project.ID, true)
		require.NoError(t, err)
		var stored []life.Point
		require.NoError(t, f.db.Where("target_id = ? AND available = ?", f.project.ID, true).Order("created_at DESC, id DESC").Find(&stored).Error)
		plan := life.PlanRetention(stored, nil)
		require.Empty(t, plan.Deletions)
		for _, row := range rows {
			expected, ok := plan.Reasons[row.ID]
			if ok {
				require.NotNil(t, row.ProtectionReason)
				require.Equal(t, expected, *row.ProtectionReason)
			} else {
				require.Nil(t, row.ProtectionReason)
			}
			if row.ID == target {
				require.Equal(t, dto.RestoreTarget, *row.ProtectionReason)
			}
			for _, point := range stored {
				if point.ID == row.ID {
					require.Equal(t, point.FailedAttempt, row.FailedAttempt)
				}
			}
		}
	}
}

func TestRestorePointHostProjectionReopensStoredGroups(t *testing.T) {
	f := newLifecycleFixture(t)
	snapshot := f.establishBaseline()
	for _, target := range []string{f.project.ID, "orphan-stack"} {
		for i := 0; i < 2; i++ {
			saved := *snapshot
			saved.TargetID = target
			saved.RegistrationID = "old-registration"
			point, err := f.service.lifecycleStore().Prepare(t.Context(), &saved, uuid.NewString(), "", "settled")
			require.NoError(t, err)
			require.NoError(t, f.service.lifecycleStore().Available(t.Context(), point.ID))
			require.NoError(t, f.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
		}
	}
	// Construct a fresh projection service over a separately reopened SQLite store.
	var paths []struct{ Name, File string }
	require.NoError(t, f.db.Raw("PRAGMA database_list").Scan(&paths).Error)
	path := ""
	for _, entry := range paths {
		if entry.Name == "main" {
			path = entry.File
		}
	}
	require.NotEmpty(t, path)
	require.NoError(t, f.db.Close())
	reopened, err := database.Initialize(t.Context(), sqlitestore.URL(path), database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, reopened.Close()) })
	reader := &ProjectService{db: reopened, settingsService: f.service.settingsService}
	rows, err := reader.ListImageRestorePoints(t.Context(), "", true)
	require.NoError(t, err)
	require.Len(t, rows, 4)
	anchors := map[string]int{}
	for _, row := range rows {
		require.False(t, row.Available)
		if row.ProtectionReason != nil {
			require.Equal(t, dto.HealthyBaseline, *row.ProtectionReason)
			anchors[row.TargetID]++
		}
	}
	require.Equal(t, map[string]int{f.project.ID: 1, "orphan-stack": 1}, anchors)
}
