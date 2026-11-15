// Source: docklet-next/issues/440
// Revision: 92ca1a3ea43cfdc211ca561a968e09c8e8f747b1
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original stack observation adapter, effect accounting and scheduled candidate filtering.

package project

import (
	"context"
	"encoding/json"
	"errors"
	compose "github.com/compose-spec/compose-go/v2/types"
	"slices"
	"strings"
	"time"

	"github.com/distribution/reference"
	life "github.com/florianfinn/docklet-next/backend/internal/project/children/lifecycle"
	"github.com/florianfinn/docklet-next/backend/internal/stability"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker/compat"
)

type lifecycleExecutionKey struct{}
type lifecycleExecution struct {
	observed bool
	// hadEffect changes only after the durable Effect transaction returns nil.
	hadEffect     bool
	enteredDeploy bool
	// A helper attempt supersedes rebuild reconciliation, even when refused.
	recoveryRan     bool
	recoveryChanged bool
	point           life.Point
	recoveryCancel  context.Context
}

func lifecycleExecutionFrom(ctx context.Context) *lifecycleExecution {
	value, _ := ctx.Value(lifecycleExecutionKey{}).(*lifecycleExecution)
	return value
}
func (s *ProjectService) lifecycleEffect(ctx context.Context, run string) error {
	if err := s.lifecycleStore().Effect(ctx, run); err != nil {
		return err
	}
	if execution := lifecycleExecutionFrom(ctx); execution != nil {
		execution.hadEffect = true
	}
	return nil
}
func lifecycleMarkRecoveryEffect(ctx context.Context) {
	if execution := lifecycleExecutionFrom(ctx); execution != nil && execution.recoveryRan {
		execution.recoveryChanged = true
	}
}
func lifecycleBeforeRecoveryEffect(ctx context.Context) error {
	if execution := lifecycleExecutionFrom(ctx); execution != nil && execution.recoveryCancel != nil {
		return execution.recoveryCancel.Err()
	}
	return nil
}

// UseObservationClock gives tests a clock without exposing it in configuration.
func (s *ProjectService) UseObservationClock(clock stability.Clock) { s.observationClock = clock }
func (s *ProjectService) observedClock() stability.Clock {
	if s.observationClock != nil {
		return s.observationClock
	}
	return stability.RealClock{}
}

type lifecycleEngine struct{ c *client.Client }

func (e lifecycleEngine) List(ctx context.Context, project, service string) ([]stability.Instance, error) {
	listed, err := e.c.ContainerList(ctx, client.ContainerListOptions{All: true, Filters: make(client.Filters).Add("label", "com.docker.compose.project="+project).Add("label", "com.docker.compose.service="+service)})
	if err != nil {
		return nil, err
	}
	out := []stability.Instance{}
	for _, item := range listed.Items {
		if item.Labels["com.docker.compose.project"] == project && item.Labels["com.docker.compose.service"] == service {
			out = append(out, stability.Instance{ID: item.ID, OneOff: strings.EqualFold(item.Labels["com.docker.compose.oneoff"], "true")})
		}
	}
	return out, nil
}
func (e lifecycleEngine) Inspect(ctx context.Context, id string) (stability.Container, error) {
	response, err := compat.ContainerInspectWithCompatibility(ctx, e.c, id, client.ContainerInspectOptions{})
	if err != nil {
		return stability.Container{}, err
	}
	c := response.Container
	result := stability.Container{ID: c.ID, RestartCount: c.RestartCount, State: "exited"}
	if c.Config != nil {
		result.OneOff = strings.EqualFold(c.Config.Labels["com.docker.compose.oneoff"], "true")
	}
	if c.State == nil {
		return result, nil
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
	return result, nil
}
func (s *ProjectService) observeLifecycle(ctx context.Context, snap *life.Snapshot, recovery bool, services []string) stability.Verdict {
	model, err := lifecycleDeployModel(snap, services)
	if err != nil {
		return stability.Verdict{Code: stability.CodeUnstable}
	}
	required := []string{}
	for key := range model.Services {
		if !recovery || snap.Services[key].Running {
			required = append(required, key)
		}
	}
	slices.Sort(required)
	set, err := stability.NewStackSet(model, required)
	if err != nil {
		return stability.Verdict{Code: stability.CodeUnstable}
	}
	c, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return stability.Verdict{Code: stability.CodeUnstable}
	}
	clock := s.observedClock()
	t0 := clock.Now()
	return stability.New(clock, lifecycleEngine{c}).Observe(ctx, set, t0)
}

func stackCandidate(sel dto.ImageSelection) (dto.AppliedCandidate, error) {
	named, err := reference.ParseNormalizedNamed(sel.Reference)
	if err != nil || sel.Service == "" || sel.Platform == "" || !dto.ValidManifest(sel.Digest) {
		return dto.AppliedCandidate{}, lifecycleFailure("update.invalid-target")
	}
	return dto.AppliedCandidate{Service: sel.Service, Repository: reference.TrimNamed(named).Name(), Platform: sel.Platform, Manifest: sel.Digest}, nil
}
func appliedStackCandidates(ctx context.Context, selections []dto.ImageSelection, result *dto.LifecycleResult) error {
	if dto.ObservedExecutionFromContext(ctx).Mode != "scheduled" {
		return nil
	}
	if len(selections) > dto.MaxAppliedCandidates {
		return lifecycleFailure("update.invalid-target")
	}
	applied := []dto.AppliedCandidate{}
	for _, sel := range selections {
		candidate, err := stackCandidate(sel)
		if err != nil {
			return err
		}
		applied = append(applied, candidate)
	}
	body, err := json.Marshal(applied)
	if err != nil || len(body) > dto.MaxRecoveryEvidenceBytes-2048 {
		return lifecycleFailure("update.invalid-target")
	}
	result.Applied = applied
	return nil
}
func filterLifecycleCandidates(ctx context.Context, current *life.Snapshot, intent *dto.LifecycleIntent) (*dto.LifecycleIntent, error) {
	observed := dto.ObservedExecutionFromContext(ctx)
	if observed.Mode != "scheduled" || len(observed.Suppressed) == 0 {
		return intent, nil
	}
	filtered := *intent
	filtered.Request = intent.Request.Clone()
	filtered.Selections = []dto.ImageSelection{}
	for _, sel := range intent.Selections {
		if !lifecycleServiceSelected(intent.Request.SelectedServices(), sel.Service) {
			return nil, lifecycleFailure("update.invalid-target")
		}
		tuple, err := stackCandidate(sel)
		if err != nil {
			return nil, err
		}
		if !slices.Contains(observed.Suppressed, tuple) {
			filtered.Selections = append(filtered.Selections, sel)
		}
	}
	if len(filtered.Selections) == len(intent.Selections) {
		return intent, nil
	}
	if len(filtered.Selections) == 0 {
		return nil, lifecycleFailure("update.candidate-failed")
	}
	services := []string{}
	for _, sel := range filtered.Selections {
		services = append(services, sel.Service)
	}
	slices.Sort(services)
	filtered.Request.Service = ""
	filtered.Request.Services = slices.Compact(services)
	if err := lifecycleValidateServiceScope(current, filtered.Request.Services); err != nil {
		return nil, err
	}
	for _, sel := range filtered.Selections {
		for other, peer := range current.Services {
			if sel.Kind == "rebuild" && !slices.Contains(services, other) && equalLifecycleReference(peer.Reference, sel.SourceReference) {
				return nil, lifecycleFailure("update.service-coupled")
			}
		}
	}
	return &filtered, nil
}

func lifecycleUncertain(err error) bool {
	var uncertain *dto.UncertainImageEffect
	return errors.As(err, &uncertain)
}

// lifecycleDeployModel is shared by deployment and observation so selection,
// presence and dependency pruning have one definition.
func lifecycleDeployModel(snap *life.Snapshot, services []string) (*compose.Project, error) {
	model, err := lifecycleOrderedModel(snap)
	if err != nil {
		return nil, err
	}
	for key := range model.Services {
		if !lifecycleServiceSelected(services, key) || !snap.Services[key].Present {
			delete(model.Services, key)
		}
	}
	for key, svc := range model.Services {
		for dependency := range svc.DependsOn {
			if _, present := model.Services[dependency]; !present {
				delete(svc.DependsOn, dependency)
			}
		}
		model.Services[key] = svc
	}
	return model, nil
}
