// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed container update: proof and observation before the original is removed, compensation and verification of the restored original.

package container

import (
	"context"
	"errors"
	"time"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/stability"
	dto "github.com/florianfinn/docklet-next/types/container"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	updaterDTO "github.com/florianfinn/docklet-next/types/updater"
	dockercontainer "github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

// observedRecreateBudget keeps the effect budget for the effects and the tail
// after a passed observation; the observer and compensation get their own.
const observedRecreateBudget = recreateEffectTimeout + stability.Window + stability.Grace + 30*time.Second

type observedRecreateKey struct{}

// observedRecreate follows one observed image update through the journal
// sequence. The journal stays the recovery record; this value only carries what
// the live sequence proved back to the update wrapper.
type observedRecreate struct {
	clock stability.Clock
	// deferred is false for a running AutoRemove original, which uses the
	// manual journal order because Docker removes it when it stops.
	deferred bool
	// journaled is set once the first intent committed; later failures may
	// follow a workload effect.
	journaled bool
	// failure is the proof or observation code that led to compensation.
	failure string
}

var errObservedContinue = errors.New("observed recreate continues at started")

func observedRecreateFrom(ctx context.Context) *observedRecreate {
	watch, _ := ctx.Value(observedRecreateKey{}).(*observedRecreate)
	return watch
}

// deferredRecreateFrom returns the observation only when the original is kept
// until the replacement is proven; startup recovery never carries one.
func deferredRecreateFrom(ctx context.Context) *observedRecreate {
	if watch := observedRecreateFrom(ctx); watch != nil && watch.deferred {
		return watch
	}
	return nil
}

// UseObservationClock replaces the real clock of later observed updates; tests
// use it to run the observation window without waiting.
func (s *ContainerService) UseObservationClock(clock stability.Clock) { s.observationClock = clock }

func (s *ContainerService) observedClock() stability.Clock {
	if s.observationClock != nil {
		return s.observationClock
	}
	return stability.RealClock{}
}

// recreateEngine inspects exact container IDs for the stability observer.
type recreateEngine struct{ c *client.Client }

func (e recreateEngine) List(context.Context, string, string) ([]stability.Instance, error) {
	return nil, errors.New("container observation lists no compose services")
}
func (e recreateEngine) Inspect(ctx context.Context, id string) (stability.Container, error) {
	response, err := compat.ContainerInspectWithCompatibility(ctx, e.c, id, client.ContainerInspectOptions{})
	if err != nil {
		return stability.Container{}, err
	}
	return stabilityContainer(response.Container), nil
}
func stabilityContainer(c dockercontainer.InspectResponse) stability.Container {
	result := stability.Container{ID: c.ID, RestartCount: c.RestartCount, State: "exited"}
	if c.State == nil {
		return result
	}
	switch {
	case c.State.Running && !c.State.Restarting && !c.State.Paused:
		result.State = "running"
	case c.State.Status != "" && c.State.Status != "running":
		result.State = string(c.State.Status)
	}
	result.ExitCode = c.State.ExitCode
	if c.State.Health != nil {
		result.Health = string(c.State.Health.Status)
	}
	result.StartedAt, _ = time.Parse(time.RFC3339Nano, c.State.StartedAt)
	return result
}

// observe runs one full window from t0 on its own deadline, so effect time
// spent before t0 never shortens it.
func (w *observedRecreate) observe(ctx context.Context, c *client.Client, set stability.RequiredSet) stability.Verdict {
	t0 := w.clock.Now()
	ctx, cancel := w.clock.WithDeadline(context.WithoutCancel(ctx), t0.Add(stability.Window+stability.Grace))
	defer cancel()
	return stability.New(w.clock, recreateEngine{c}).Observe(ctx, set, t0)
}

// proveImageRecreate is the identity and image proof without the slot owner
// advance. It returns the observed replacement for callers that need it.
func proveImageRecreate(ctx context.Context, c *client.Client, j *RecreateJournal) (*dockercontainer.InspectResponse, error) {
	current, err := recreateInspect(ctx, c, j.NewName)
	if err != nil {
		return nil, err
	}
	if current == nil || current.ID != j.NewID {
		return current, imageDTO.Failure("update.target-changed", 409)
	}
	if j.ExpectedImageID != nil && current.Image != *j.ExpectedImageID {
		return current, refusedImageError{imageDTO.Failure("update.target-changed", 409)}
	}
	return current, nil
}

// refusedImageError is the target-changed proof failure of a replacement that
// holds the new name with its exact ID but runs another image.
type refusedImageError struct{ error }

func (e refusedImageError) Unwrap() error { return e.error }

func refusedReplacement(err error) bool {
	var refused refusedImageError
	return errors.As(err, &refused)
}

// observedBeforeRemoval names the journal phases in which an observed update
// still keeps the original and has not proven the replacement.
func observedBeforeRemoval(phase string) bool {
	return phase == "intent-create" || phase == "created" || phase == "intent-start" || phase == "started"
}

// observeReplacement proves and observes the replacement while the original is
// kept. A failure compensates with the existing journal compensation; handled
// is false only when the unchanged tail may remove the original.
func (s *ContainerService) observeReplacement(ctx context.Context, c *client.Client, j *RecreateJournal, watch *observedRecreate) (bool, error) {
	if _, err := proveImageRecreate(ctx, c, j); err != nil {
		var p *imageDTO.OperationError
		if !errors.As(err, &p) || p.Problem.Code != "update.target-changed" {
			return true, &recreateUncertain{cause: err}
		}
		watch.failure = p.Problem.Code
	} else if j.WasRunning {
		if verdict := watch.observe(ctx, c, stability.NewContainerSet(j.NewID)); !verdict.Pass {
			watch.failure = stability.CodeUnstable
		}
	}
	if watch.failure == "" {
		return false, nil
	}
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), recreateEffectTimeout)
	defer cancel()
	old, err := recreateInspect(ctx, c, j.OldID)
	if err != nil {
		return true, &recreateUncertain{cause: err}
	}
	replacement, err := recreateInspect(ctx, c, j.NewID)
	if err != nil {
		return true, &recreateUncertain{cause: err}
	}
	recovery, err := s.compensateRecreate(ctx, c, j, old, replacement)
	if err != nil {
		return true, &recreateUncertain{cause: err}
	}
	return true, dto.RecreateFailure(recovery.Code, "verify")
}

// verifyOriginal observes the original after compensation reached rolled-back.
// A stopped original has nothing to observe; compensation is its proof. The
// baseline accepts an original that compensation never had to restart.
func (s *ContainerService) verifyOriginal(ctx context.Context, c *client.Client, watch *observedRecreate, before dockercontainer.InspectResponse) string {
	if before.State == nil || !before.State.Running {
		return "restored"
	}
	started, _ := time.Parse(time.RFC3339Nano, before.State.StartedAt)
	set := stability.NewContainerSetSince(before.ID, before.RestartCount, started)
	if watch.observe(ctx, c, set).Pass {
		return "restored"
	}
	return "restore-failed"
}

// observedLifecycle completes the observed evidence after the journal returned.
// Held (uncertain) outcomes carry no Recovery; the run stays unknown.
func (s *ContainerService) observedLifecycle(ctx context.Context, c *client.Client, watch *observedRecreate, before dockercontainer.InspectResponse, result *updaterDTO.LifecycleResult, err error) error {
	var uncertain *recreateUncertain
	var problem *dto.Problem
	if err == nil || errors.As(err, &uncertain) || (errors.As(err, &problem) && problem.Code == "container.recreate-ambiguous") {
		return err
	}
	recovered := problem != nil && problem.Code == "container.recreate-recovered"
	switch {
	case !watch.deferred && watch.journaled:
		result.Recovery = &updaterDTO.RecoveryResult{Outcome: "unavailable", Code: "container.auto-remove"}
	case recovered:
		result.Recovery = &updaterDTO.RecoveryResult{Outcome: s.verifyOriginal(ctx, c, watch, before)}
		switch watch.failure {
		case "update.target-changed":
			return imageDTO.Failure(watch.failure, 409)
		case stability.CodeUnstable:
			return imageDTO.Failure(watch.failure, 500)
		}
		return imageDTO.Failure("update.deploy-failed", 500)
	}
	return err
}

// appliedContainerCandidate is the failed-candidate tuple of a standalone
// container: no service, the repository without tag, the pullable manifest.
func appliedContainerCandidate(ref, platform, manifest string) (updaterDTO.AppliedCandidate, bool) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil || platform == "" || !updaterDTO.ValidManifest(manifest) {
		return updaterDTO.AppliedCandidate{}, false
	}
	return updaterDTO.AppliedCandidate{Repository: reference.TrimNamed(named).Name(), Platform: platform, Manifest: manifest}, true
}
