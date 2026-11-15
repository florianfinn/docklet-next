// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed container update coverage over the stateful Docker producer and real journal store.

package container

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/stability"
	"github.com/florianfinn/docklet-next/backend/internal/stability/stabilitytest"
	dto "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	dc "github.com/moby/moby/api/types/container"
	"github.com/stretchr/testify/require"
)

const observedManifest = "sha256:9999999999999999999999999999999999999999999999999999999999999999"

type observedRun struct {
	ctx       context.Context
	clock     *stabilitytest.Clock
	lifecycle *updaterDTO.LifecycleResult
	recreated *dto.RecreateResult
}

func observedUpdate(t *testing.T, s *ContainerService, mode string, suppressed ...updaterDTO.SuppressedCandidate) *observedRun {
	t.Helper()
	run := &observedRun{clock: stabilitytest.NewClock()}
	ctx := updateContext(t)
	if mode != "" {
		ctx = updaterDTO.WithObservedExecution(ctx, updaterDTO.ObservedExecution{Mode: mode, Suppressed: suppressed})
	}
	s.UseObservationClock(run.clock)
	ctx = updaterDTO.WithResultSink(ctx, func(result updaterDTO.LifecycleResult) { owned := result; run.lifecycle = &owned })
	run.ctx = dto.WithRecreateResultSink(ctx, func(result dto.RecreateResult) { owned := result; run.recreated = &owned })
	return run
}

// exitAfter stops the exact container once the observation clock passed the offset.
func exitAfter(clock *stabilitytest.Clock, id string, offset time.Duration) func(string, *dc.InspectResponse) {
	return func(inspected string, item *dc.InspectResponse) {
		if inspected == id && clock.Elapsed() >= offset {
			item.State.Running, item.State.Status, item.State.ExitCode = false, "exited", 1
		}
	}
}

func lifecycleCode(t *testing.T, err error) string {
	t.Helper()
	return imageDTO.Neutral(err, "unexpected", 500).Problem.Code
}
func requireOriginalBack(t *testing.T, d *updateDaemon) {
	t.Helper()
	original, ok := d.containers[updateOldID]
	require.True(t, ok, "original container removed")
	require.Equal(t, "/app", original.Name)
	require.True(t, original.State.Running)
	require.NotContains(t, d.containers, updateNewID)
}
func requireRecovery(t *testing.T, run *observedRun, outcome string) {
	t.Helper()
	require.NotNil(t, run.lifecycle)
	require.NotNil(t, run.lifecycle.Recovery)
	require.Equal(t, outcome, run.lifecycle.Recovery.Outcome)
	require.False(t, run.lifecycle.Verified)
}

// C-1: an unstable replacement is compensated while the original was kept.
func TestObservedContainerUnstableReplacementRestoresOriginal(t *testing.T) {
	for _, mode := range []string{"manual", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			s, d, p, _ := updateFixture(t)
			if mode == "scheduled" {
				s.containerImages = &candidatePort{updateImagePort: p}
			}
			run := observedUpdate(t, s, mode)
			d.inspect = exitAfter(run.clock, updateNewID, 10*time.Second)
			err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
			require.Equal(t, "update.unstable", lifecycleCode(t, err))
			require.Equal(t, "update.unstable", run.lifecycle.Code)
			requireRecovery(t, run, "restored")
			require.Nil(t, run.recreated)
			requireOriginalBack(t, d)
			// The replacement fails at 10 s; the restored original then gets its own window.
			require.Equal(t, 10*time.Second+stability.Window, run.clock.Elapsed())
			slot, e := s.previousImage(t.Context(), "app")
			require.NoError(t, e)
			require.Equal(t, updateOldID, slot.OwnerContainerID)
		})
	}
}

// A stable replacement passes the full window before the original is removed.
func TestObservedContainerStableReplacementSucceeds(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := observedUpdate(t, s, "manual")
	keptThroughWindow := false
	d.inspect = func(id string, _ *dc.InspectResponse) {
		if _, present := d.containers[updateOldID]; id == updateNewID && present && run.clock.Elapsed() >= stability.Window {
			keptThroughWindow = true
		}
	}
	require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
	require.True(t, keptThroughWindow, "original removed before the window ended")
	require.GreaterOrEqual(t, run.clock.Elapsed(), stability.Window)
	require.NotContains(t, d.containers, updateOldID)
	require.NotNil(t, run.recreated)
	require.Equal(t, updateNewID, run.recreated.NewID)
	require.True(t, run.lifecycle.Verified)
	require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
}

// C-2: a replacement on another image is compensated before the original is removed.
func TestObservedContainerProofFailsBeforeRemoval(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	d.createSlot = func() { d.tag = updateOtherImage }
	run := observedUpdate(t, s, "manual")
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", lifecycleCode(t, err))
	requireRecovery(t, run, "restored")
	require.Nil(t, run.recreated)
	requireOriginalBack(t, d)
}

// C-3: a restored original that fails its own observation is restore-failed,
// after exactly one compensation.
func TestObservedContainerUnstableOriginalIsRestoreFailed(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := observedUpdate(t, s, "manual")
	var restartedAt time.Duration
	replacement := exitAfter(run.clock, updateNewID, 10*time.Second)
	d.inspect = func(id string, item *dc.InspectResponse) {
		replacement(id, item)
		if id == updateOldID && d.counts["start"] >= 2 {
			if restartedAt == 0 {
				restartedAt = run.clock.Elapsed()
			}
			if run.clock.Elapsed() >= restartedAt+10*time.Second {
				item.State.Running, item.State.Status = false, "exited"
			}
		}
	}
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.unstable", lifecycleCode(t, err))
	requireRecovery(t, run, "restore-failed")
	require.Equal(t, 2, d.counts["rename"], "one rename away and one back")
	require.Equal(t, 2, d.counts["start"], "replacement start and one compensation start")
	require.Contains(t, d.containers, updateOldID)
}

// C-3b: a compensation effect failure stays unknown and held; no recovery claim.
func TestObservedContainerCompensationFailureIsUnknown(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := observedUpdate(t, s, "manual")
	d.inspect = exitAfter(run.clock, updateNewID, 10*time.Second)
	d.fail = "remove"
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	var uncertain interface{ RuntimeUncertain() bool }
	require.True(t, errors.As(err, &uncertain) && uncertain.RuntimeUncertain())
	require.Nil(t, run.lifecycle.Recovery)
	var journal RecreateJournal
	require.NoError(t, s.operations.db.First(&journal, "run_id=?", run.lifecycle.RunID).Error)
	require.Equal(t, "ambiguous", journal.Phase)
}

// C-5 / v5-3: an errored start the daemon completed continues the observed tail;
// an errored start that never ran compensates. Neither removes the original first.
func TestObservedContainerLiveStartErrorDispatch(t *testing.T) {
	t.Run("replacement running", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		run := observedUpdate(t, s, "manual")
		d.failAfter = "start"
		d.inspect = exitAfter(run.clock, updateNewID, 10*time.Second)
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.unstable", lifecycleCode(t, err))
		requireRecovery(t, run, "restored")
		requireOriginalBack(t, d)
	})
	t.Run("replacement stopped", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		run := observedUpdate(t, s, "manual")
		d.fail = "start"
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.deploy-failed", lifecycleCode(t, err))
		requireRecovery(t, run, "restored")
		requireOriginalBack(t, d)
	})
}

// v5-3: a stopped original whose errored create completed is proven before it is
// removed; a replacement on another image compensates instead.
func TestObservedContainerStoppedOriginalProvenBeforeRemoval(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	original := d.containers[updateOldID]
	original.State.Running = false
	d.containers[updateOldID] = original
	d.createSlot = func() { d.tag = updateOtherImage }
	d.failAfter = "create"
	run := observedUpdate(t, s, "manual")
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", lifecycleCode(t, err))
	requireRecovery(t, run, "restored")
	kept, ok := d.containers[updateOldID]
	require.True(t, ok, "stopped original removed before the proof")
	require.Equal(t, "/app", kept.Name)
	require.NotContains(t, d.containers, updateNewID)
}

// C-6 / v5-2: a running AutoRemove original keeps the manual journal order; a
// failure after Docker removed it reports recovery unavailable and keeps B.
func TestObservedContainerRunningAutoRemove(t *testing.T) {
	autoRemove := func(d *updateDaemon) {
		original := d.containers[updateOldID]
		original.HostConfig.AutoRemove = true
		d.containers[updateOldID] = original
	}
	t.Run("success", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		autoRemove(d)
		run := observedUpdate(t, s, "manual")
		d.inspect = exitAfter(run.clock, updateNewID, 10*time.Second)
		require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
		require.Zero(t, run.clock.Elapsed(), "no observation on the manual order")
		require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
		require.NotNil(t, run.recreated)
	})
	t.Run("failure after removal", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		autoRemove(d)
		d.createSlot = func() { d.tag = updateOtherImage }
		run := observedUpdate(t, s, "manual")
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.target-changed", lifecycleCode(t, err))
		requireRecovery(t, run, "unavailable")
		require.Equal(t, "container.auto-remove", run.lifecycle.Recovery.Code)
		require.NotContains(t, d.containers, updateOldID)
		require.Contains(t, d.containers, updateNewID, "the only copy is kept")
		require.NotNil(t, run.recreated, "origin follows the kept replacement")
	})
}

// C-7 / O-10: the plain manual update never observes and keeps its order.
func TestPlainManualContainerUpdateNeverObserves(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := observedUpdate(t, s, "")
	d.inspect = exitAfter(run.clock, updateNewID, 0)
	require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
	require.Zero(t, run.clock.Elapsed())
	require.Nil(t, run.lifecycle.Recovery)
	require.True(t, run.lifecycle.Verified)
	require.NotContains(t, d.containers, updateOldID)
	require.NotNil(t, run.recreated)
}

// C-10: effect time before t0 never shortens the window.
func TestObservedContainerSlowEffectsKeepFullWindow(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	run := observedUpdate(t, s, "manual")
	d.createSlot = func() { run.clock.Jump(6 * time.Minute) }
	require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
	require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
	require.GreaterOrEqual(t, run.clock.Elapsed(), 6*time.Minute+stability.Window)
}

// DV-6: the restored original is verified with no baseline input from the caller:
// a compensation start resets its counter; an original never stopped keeps it.
func TestObservedContainerOriginalRestartCount(t *testing.T) {
	seed := func(d *updateDaemon) {
		original := d.containers[updateOldID]
		original.RestartCount = 3
		original.State.StartedAt = time.Now().Add(-time.Hour).UTC().Format(time.RFC3339Nano)
		d.containers[updateOldID] = original
	}
	t.Run("restarted by compensation", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		seed(d)
		run := observedUpdate(t, s, "manual")
		d.inspect = exitAfter(run.clock, updateNewID, 10*time.Second)
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.unstable", lifecycleCode(t, err))
		requireRecovery(t, run, "restored")
		require.Zero(t, d.containers[updateOldID].RestartCount)
	})
	t.Run("never stopped", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		seed(d)
		d.fail = "rename"
		run := observedUpdate(t, s, "manual")
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.deploy-failed", lifecycleCode(t, err))
		requireRecovery(t, run, "restored")
		require.Equal(t, 3, d.containers[updateOldID].RestartCount)
	})
	t.Run("restarted by policy", func(t *testing.T) {
		s, d, _, _ := updateFixture(t)
		seed(d)
		d.fail = "rename"
		run := observedUpdate(t, s, "manual")
		d.inspect = func(id string, item *dc.InspectResponse) {
			if id == updateOldID && d.counts["rename"] > 0 {
				item.RestartCount, item.State.StartedAt = 4, time.Now().UTC().Format(time.RFC3339Nano)
			}
		}
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.deploy-failed", lifecycleCode(t, err))
		requireRecovery(t, run, "restore-failed")
	})
}

type candidatePort struct{ *updateImagePort }

func (p *candidatePort) CandidateContainerImage(context.Context, dc.InspectResponse, string) (string, string, string, error) {
	return p.expected, "linux/amd64", observedManifest, nil
}

// #441: a scheduled run refuses a suppressed candidate before any effect and
// reports the applied tuple otherwise.
func TestScheduledContainerSuppressedCandidate(t *testing.T) {
	tuple := updaterDTO.AppliedCandidate{Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: observedManifest}
	t.Run("suppressed", func(t *testing.T) {
		s, d, p, _ := updateFixture(t)
		s.containerImages = &candidatePort{updateImagePort: p}
		run := observedUpdate(t, s, "scheduled", tuple)
		err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
		require.Equal(t, "update.candidate-failed", lifecycleCode(t, err))
		require.Equal(t, "not-needed", run.lifecycle.Recovery.Outcome)
		require.Empty(t, d.counts, "no image or workload effect")
	})
	t.Run("other candidate", func(t *testing.T) {
		s, _, p, _ := updateFixture(t)
		s.containerImages = &candidatePort{updateImagePort: p}
		other := tuple
		other.Manifest = "sha256:1111111111111111111111111111111111111111111111111111111111111111"
		run := observedUpdate(t, s, "scheduled", other)
		require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
		require.Equal(t, []updaterDTO.AppliedCandidate{tuple}, run.lifecycle.Applied)
	})
	t.Run("manual with check ignores suppression", func(t *testing.T) {
		s, _, p, _ := updateFixture(t)
		s.containerImages = &candidatePort{updateImagePort: p}
		run := observedUpdate(t, s, "manual", tuple)
		require.NoError(t, s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{}))
		require.Empty(t, run.lifecycle.Applied)
	})
}

// #409.2: a refused success records the replacement so the origin follows it.
func TestRefusedContainerUpdateRecordsReplacement(t *testing.T) {
	s, d, _, _ := updateFixture(t)
	d.createSlot = func() { d.tag = updateOtherImage }
	run := observedUpdate(t, s, "")
	err := s.ExecuteContainerImage(run.ctx, updateOldID, "app", false, user.Actor{})
	require.Equal(t, "update.target-changed", lifecycleCode(t, err))
	require.NotNil(t, run.recreated)
	require.Equal(t, dto.RecreateResult{OldID: updateOldID, NewID: updateNewID, OldName: "app", NewName: "app"}, *run.recreated)
}
