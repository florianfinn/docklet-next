// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package activity

import (
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/kv"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	dto "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestHostUpdateExclusionBothDirections(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	service := NewActivityService(db, nil)
	store := core.NewStore(db)
	run, err := service.StartGatedActivity(t.Context(), GateRequest{EnvironmentID: "0", Operation: "deploy", Scope: "project:example", Type: dto.TypeProjectDeploy})
	require.NoError(t, err)
	require.ErrorContains(t, CheckHostUpdateAdmissionTx(db.DB, "0"), "host.busy", "workload must exclude update")
	_, err = service.CompleteActivity(run.WorkContext, run.Run.ID, dto.StatusSuccess, "done", nil)
	require.NoError(t, err)
	require.NoError(t, CheckHostUpdateAdmissionTx(db.DB, "0"))
	op := core.Operation{OpID: "update-operation", Role: "hub", EnvironmentID: "0", TargetRef: "example.com/hub:1.0.1", TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	_, _, err = store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	_, err = service.StartGatedActivity(t.Context(), GateRequest{EnvironmentID: "0", Operation: "deploy", Scope: "project:other", Type: dto.TypeProjectDeploy})
	require.ErrorContains(t, err, "host.busy", "update must exclude workload")
	require.ErrorContains(t, runtimeCheckRestartTx(db.DB, "0"), "host.busy", "every runtime admission must share update exclusion")
}
func TestRepairKeepsDurableSelfUpdateOwner(t *testing.T) {
	db := setupActivityServiceTestDBInternal(t)
	require.NoError(t, db.AutoMigrate(&kv.KVEntry{}))
	s := NewActivityService(db, nil)
	run, err := s.StartActivity(t.Context(), StartActivityRequest{EnvironmentID: "0", Type: dto.TypeHubUpdate})
	require.NoError(t, err)
	store := core.NewStore(db)
	op := core.Operation{OpID: "delayed-result", Role: "hub", RunID: run.ID, EnvironmentID: "0", TargetRef: "example.com/hub:1.0.1", TargetVersion: "1.0.1", Deadline: time.Now().Add(core.OperationTimeout)}
	_, _, err = store.Reserve(t.Context(), op, nil)
	require.NoError(t, err)
	repaired, err := s.RepairStartup(t.Context())
	require.NoError(t, err)
	require.Zero(t, repaired)
	var row Activity
	require.NoError(t, db.First(&row, "id=?", run.ID).Error)
	require.Equal(t, dto.StatusRunning, row.Status)
}
