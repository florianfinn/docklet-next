// Source: docklet-next/docs/design/update-policies.md
// Revision: 529a0657c534d1851b032a1b56998f15af095fbd
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Admitted restore refusal and persisted settlement after a late executor delete.

package activity

import (
	"context"
	"io"
	"strings"
	"sync/atomic"
	"testing"

	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestPointAdmittedLateDeleteSettlesRefused(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	// The executor owns a separate store: a hub admission cannot act as its lock.
	executor := b1Fixture(t)
	store := life.Store{DB: executor.store.svc.db}
	snapshot := &life.Snapshot{TargetID: "example-stack", TargetIdentity: strings.Repeat("b", 64), RegistrationID: "example-registration", SavedRevision: strings.Repeat("a", 64)}
	point, err := store.Prepare(t.Context(), snapshot, uuid.NewString(), "", "settled")
	require.NoError(t, err)
	require.NoError(t, store.Available(t.Context(), point.ID))
	require.NoError(t, executor.store.svc.db.Model(&life.Run{}).Where("point_id = ?", point.ID).Update("settled", true).Error)
	entered, resume := make(chan struct{}), make(chan struct{})
	defer func() {
		select {
		case <-resume:
		default:
			close(resume)
		}
	}()
	var effects atomic.Int32
	f := b2Fixture(t, t.Context(), func(ctx context.Context, c RuntimeCommand, _ io.Writer) error {
		close(entered)
		<-resume
		_, err := store.Prepare(ctx, snapshot, c.ProposedRunID, c.Options.Lifecycle.Request.PointID, "settled")
		result := dto.LifecycleResult{RunID: c.ProposedRunID}
		if err != nil {
			problem := imageDTO.Neutral(err, "update.persistence-failed", 500)
			result.Code = problem.Problem.Code
			result.HTTPStatus = problem.Status
		} else {
			effects.Add(1)
		}
		dto.RecordLifecycleResult(ctx, result)
		return err
	})
	c := imageCommand(f.command(), RuntimeRestoreProject)
	c.Options.Lifecycle.Request.PointID = point.ID
	admitted, err := f.service.Admit(t.Context(), c)
	require.NoError(t, err)
	b1Await(t, entered)
	var receipt RuntimeReceipt
	require.NoError(t, f.store.svc.db.First(&receipt, "run_id = ?", admitted.ActivityID).Error)
	require.Nil(t, receipt.SettledAt)
	var intent RuntimeImageIntent
	require.NoError(t, f.store.svc.db.First(&intent, "run_id = ?", admitted.ActivityID).Error)
	require.Contains(t, intent.Payload, point.ID)
	require.NoError(t, store.Delete(t.Context(), point.ID))
	close(resume)
	settled, err := b2Wait(t, f, c, admitted.ActivityID)
	require.NoError(t, err)
	require.Equal(t, activityDTO.StatusFailed, settled.Status)
	require.NotNil(t, settled.ImageLifecycle)
	require.Equal(t, "restore.unavailable", settled.ImageLifecycle.Code)
	require.Equal(t, 409, settled.ImageLifecycle.HTTPStatus)
	require.Zero(t, effects.Load())
	require.NoError(t, f.store.svc.db.First(&receipt, "run_id = ?", admitted.ActivityID).Error)
	require.NotNil(t, receipt.SettledAt)
	require.NotNil(t, receipt.ExecutorSettledAt)
	require.Equal(t, "failed", *receipt.ExecutorState)
	var runs int64
	require.NoError(t, executor.store.svc.db.Model(&life.Run{}).Where("selected_point_id = ?", point.ID).Count(&runs).Error)
	require.Zero(t, runs)
}
