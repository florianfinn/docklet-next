// Source: docklet-next/issues/440
// Revision: 6119f8e1c40d96a44338ae4a681bb6a5fbfa53c1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original regressions for the first S3 review: live recovered refusals, AutoRemove effect errors, a vanished replacement and joined failures.

package container

import (
	"context"
	"errors"
	"testing"
	"time"

	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
)

// R1S3-1: an errored removal whose recovery proves a replacement on another image
// keeps the failure while R still moves the origin.
func TestRecoveredRefusalKeepsFailure(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	d.createSlot = func() { d.tag = updateOtherImage }
	d.failAfter = "remove"
	run := observedUpdate(t, s, "")
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", lifecycleCode(t, err))
	require.Equal(t, "update.target-changed", run.lifecycle.Code)
	require.False(t, run.lifecycle.Verified)
	require.NotNil(t, run.recreated)
	require.Equal(t, updateNewID, run.recreated.NewID)
}

// R1S3-2: after Docker removed a running AutoRemove original, a known start
// failure keeps the replacement and reports recovery unavailable; a start the
// daemon completed is the update's result.
func TestObservedAutoRemoveStartErrors(t *testing.T) {
	for _, tc := range []struct {
		name      string
		failed    bool
		configure func(*updateDaemon)
	}{
		{"start failed", true, func(d *updateDaemon) { d.fail = "start" }},
		{"start completed", false, func(d *updateDaemon) { d.failAfter = "start" }},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s, d, p, _ := updateFixture(t)
			s.containerImages = &candidatePort{updateImagePort: p}
			original := d.containers[updateOldID]
			original.HostConfig.AutoRemove = true
			d.containers[updateOldID] = original
			tc.configure(d)
			run := observedUpdate(t, s, "scheduled")
			err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
			require.NotContains(t, d.containers, updateOldID)
			require.Contains(t, d.containers, updateNewID, "the only copy is kept")
			var journal RecreateJournal
			require.NoError(t, s.operations.db.First(&journal, "run_id=?", run.lifecycle.RunID).Error)
			require.Equal(t, "done", journal.Phase)
			require.Zero(t, run.clock.Elapsed(), "running AutoRemove keeps the unobserved order")
			if !tc.failed {
				require.NoError(t, err)
				require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
				require.NotNil(t, run.recreated)
				return
			}
			var uncertain interface{ RuntimeUncertain() bool }
			require.False(t, errors.As(err, &uncertain))
			require.Equal(t, "update.deploy-failed", run.lifecycle.Code)
			requireRecovery(t, run, "unavailable")
			require.Equal(t, "container.auto-remove", run.lifecycle.Recovery.Code)
			require.Nil(t, run.recreated)
		})
	}
}

// R1S3-3: a live observed recovery that finds the original but not the recorded
// replacement compensates the original instead of holding the run.
func TestObservedVanishedReplacementCompensates(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := updateContext(t)
	runID := updaterDTO.ExecutionIdentityFromContext(run).RunID
	journal := RecreateJournal{RunID: runID, OldID: updateOldID, OldName: "app", BackupName: "app-docklet-recreate-" + runID, NewID: updateNewID, NewName: "app", Phase: "intent-start", WasRunning: true, UpdatedAt: time.Now().UTC(), ExpectedImageID: new(updateNewImage)}
	require.NoError(t, s.operations.db.Create(&journal).Error)
	original := d.containers[updateOldID]
	original.Name, original.State.Running = "/"+journal.BackupName, false
	d.containers[updateOldID] = original
	c, err := s.dockerService.GetClient(run)
	require.NoError(t, err)
	ctx := context.WithValue(run, observedRecreateKey{}, &observedRecreate{clock: s.observedClock(), deferred: true, journaled: true})
	recovery, err := s.recoverRecreate(ctx, c, &journal)
	require.NoError(t, err)
	require.Equal(t, "container.recreate-recovered", recovery.Code)
	requireOriginalBack(t, d)
	require.Equal(t, "rolled-back", journal.Phase)
}

// R1S3-4: a failure after a passed observation (owner advance) is joined; the
// worker keeps it against a later cancellation.
func TestObservedPostObservationFailureIsJoined(t *testing.T) {
	s, d, _, db := updateFixture(t)
	require.NoError(t, db.Exec(`CREATE TRIGGER fail_owner BEFORE UPDATE OF owner_container_id ON container_previous_images BEGIN SELECT RAISE(ABORT, 'synthetic owner failure'); END`).Error)
	run := observedUpdate(t, s, "manual")
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Error(t, err)
	var joined interface{ RuntimeJoined() bool }
	require.True(t, errors.As(err, &joined) && joined.RuntimeJoined())
	require.Equal(t, "update.deploy-failed", run.lifecycle.Code)
	require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
	require.Nil(t, run.recreated)
	require.NotContains(t, d.containers, updateOldID)
	require.Contains(t, d.containers, updateNewID)

	// A refusal before the first journal write stays an ordinary, cancellable failure.
	s2, _, p2, _ := updateFixture(t)
	s2.containerImages = &candidatePort{updateImagePort: p2}
	tuple := updaterDTO.AppliedCandidate{Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: observedManifest}
	refused := observedUpdate(t, s2, "scheduled", tuple)
	err = s2.ExecuteContainerImage(refused.ctx, updateOldID, "app", false, user.Actor{})
	require.Error(t, err)
	require.False(t, errors.As(err, &joined))
}
