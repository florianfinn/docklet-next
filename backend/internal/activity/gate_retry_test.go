// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Fault-injected stale lookup verifies unique-conflict admission retries.

package activity

import (
	"errors"
	"testing"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestGateUniqueConflictRetriesLookupWithoutWorker(t *testing.T) {
	for _, sameKey := range []bool{true, false} {
		t.Run(map[bool]string{true: "join", false: "busy"}[sameKey], func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			owner := NewActivityService(db, nil)
			request := GateRequest{Operation: "deploy", Scope: "stack:example", Type: activitytypes.TypeProjectDeploy}
			started, err := owner.StartGatedActivity(t.Context(), request)
			require.NoError(t, err)
			t.Cleanup(func() {
				_, _ = owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "done", nil)
			})
			if !sameKey {
				request.Operation = "update"
			}
			lookups, conflicts := 0, 0
			// Hide the first active lookup to reach the defensive unique-conflict path.
			require.NoError(t, db.Callback().Query().After("gorm:query").Register("test:stale_gate_lookup", func(tx *gorm.DB) {
				if tx.Statement.Table == "activities" {
					lookups++
					if lookups == 1 {
						tx.Error = gorm.ErrRecordNotFound
						tx.RowsAffected = 0
					}
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Query().Remove("test:stale_gate_lookup") })
			require.NoError(t, db.Callback().Create().After("gorm:create").Register("test:gate_conflict", func(tx *gorm.DB) {
				if tx.Statement.Table == "activities" && tx.Error != nil {
					conflicts++
				}
			}))
			t.Cleanup(func() { _ = db.Callback().Create().Remove("test:gate_conflict") })
			contender := NewActivityService(db, nil)
			answer, err := contender.StartGatedActivity(t.Context(), request)
			if sameKey {
				require.NoError(t, err)
				require.Equal(t, string(activitytypes.CodeRunJoined), answer.Code)
				require.Equal(t, started.Run.ID, answer.Run.ID)
			} else {
				var conflict *GateConflict
				require.True(t, errors.As(err, &conflict))
				require.Equal(t, string(activitytypes.CodeRunBusy), conflict.Code)
				require.Equal(t, started.Run.ID, conflict.RunID)
			}
			require.Nil(t, answer.WorkContext)
			require.Empty(t, contender.running)
			require.Equal(t, 2, lookups)
			require.Equal(t, 1, conflicts)
			var count int64
			require.NoError(t, db.Model(&Activity{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
		})
	}
}
