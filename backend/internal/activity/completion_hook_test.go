// Source: docklet-next/issues/467
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-17
// License: Apache-2.0
// Deviations: Original proofs that completion hook writes share the terminal transaction.

package activity

import (
	"errors"
	"testing"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestCompletionHookCommitsWithTerminalStatus(t *testing.T) {
	for _, fails := range []bool{false, true} {
		t.Run(map[bool]string{false: "commits", true: "rolls-back"}[fails], func(t *testing.T) {
			db := setupActivityServiceTestDBInternal(t)
			owner := NewActivityService(db, nil)
			request := GateRequest{Operation: "backup", Scope: "system-backup", Type: activitytypes.TypeResourceAction}
			started, err := owner.StartGatedActivity(t.Context(), request)
			require.NoError(t, err)
			var seen activitytypes.Status
			hook := func(tx *gorm.DB, status activitytypes.Status) error {
				seen = status
				if err := tx.Create(&ActivityMessage{ActivityID: started.Run.ID, Message: "dependent write", Level: activitytypes.MessageLevelInfo}).Error; err != nil {
					return err
				}
				if fails {
					return errors.New("controlled hook failure")
				}
				return nil
			}
			_, err = owner.CompleteActivityWithHook(started.WorkContext, started.Run.ID, activitytypes.StatusSuccess, "done", nil, hook)
			require.Equal(t, activitytypes.StatusSuccess, seen)
			var dependent int64
			require.NoError(t, db.Model(&ActivityMessage{}).Where("activity_id = ? AND message = ?", started.Run.ID, "dependent write").Count(&dependent).Error)
			var stored Activity
			require.NoError(t, db.First(&stored, "id = ?", started.Run.ID).Error)
			if fails {
				require.ErrorIs(t, err, ErrCompletionHook)
				require.Zero(t, dependent, "a failed hook must roll back its own writes")
				require.Equal(t, activitytypes.StatusRunning, stored.Status, "a failed hook must roll back the terminal status")
				_, err = owner.CompleteActivity(started.WorkContext, started.Run.ID, activitytypes.StatusFailed, "failed", nil)
				require.NoError(t, err)
				return
			}
			require.NoError(t, err)
			require.Equal(t, int64(1), dependent)
			require.Equal(t, activitytypes.StatusSuccess, stored.Status)
			next, err := owner.StartGatedActivity(t.Context(), GateRequest{Operation: "next", Scope: "system-backup", Type: activitytypes.TypeResourceAction})
			require.NoError(t, err, "the committed hook state must coincide with the gate release")
			_, err = owner.CompleteActivity(next.WorkContext, next.Run.ID, activitytypes.StatusSuccess, "done", nil)
			require.NoError(t, err)
		})
	}
}
