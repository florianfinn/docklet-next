// Source: getarcaneapp/arcane/backend/pkg/scheduler/auto_heal_job.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Durable per-host attempts, current final eligibility and crash-safe projection for issue #296

package scheduler

import (
	"cmp"
	"context"
	"encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/autoheal"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/notification"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	scheduleutil "github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	autohealDTO "github.com/florianfinn/docklet-next/types/autoheal"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/docker"
	"go.getarcane.app/docker/compat"
	kit "go.getarcane.app/kit/pkg"
	"go.getarcane.app/sys/cgroup"
	"golang.org/x/sync/errgroup"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"time"
)

const (
	AutoHealJobName                = "auto-heal"
	autoHealDefaultSchedule        = "0 */5 * * * *"
	autoHealInspectConcurrency     = 4
	autoHealAdmissionScopeInternal = "auto-heal"
)

type autoHealPlanInternal struct {
	Containers    []container.Summary `json:"containers"`
	MaxRestarts   int                 `json:"maxRestarts"`
	WindowMinutes int                 `json:"windowMinutes"`
}
type autoHealBaselineInternal struct {
	StartedAt    string `json:"startedAt"`
	RestartCount int    `json:"restartCount"`
}
type AutoHealOptions struct {
	DB     *database.DB
	Config *config.Config
	Busy   func(context.Context, container.InspectResponse) (bool, error)
	Now    func() time.Time
}
type AutoHealJob struct {
	dockerClientService *dockerInternal.DockerClientService
	settingsService     *settings.SettingsService
	admissionGate       *runs.Admission
	store               *autoheal.Store
	busy                func(context.Context, container.InspectResponse) (bool, error)
	selfIDOnce          sync.Once
	selfID              string
	getDockerClient     func() (*client.Client, error)
	listContainers      func(context.Context, *client.Client) ([]container.Summary, error)
	inspectContainer    func(context.Context, *client.Client, string) (container.InspectResponse, error)
	restartContainer    func(context.Context, *client.Client, string) error
	getSelfContainerID  func() (string, error)
	afterReserve        func()
}

func NewAutoHealJob(d *dockerInternal.DockerClientService, s *settings.SettingsService, _ *event.EventService, n *notification.NotificationService, a *runs.Admission, options ...AutoHealOptions) (*AutoHealJob, error) {
	if a == nil {
		return nil, errors.New("auto-heal admission unavailable")
	}
	j := &AutoHealJob{dockerClientService: d, settingsService: s, admissionGate: a}
	if len(options) > 0 {
		opt := options[0]
		j.store = &autoheal.Store{DB: opt.DB, Config: opt.Config, Now: opt.Now, Settings: s}
		j.busy = opt.Busy
		if n != nil {
			j.store.Dispatch = n.SendHostEvent
		}
	}
	return j, nil
}
func (*AutoHealJob) Name() string { return AutoHealJobName }
func (j *AutoHealJob) ShouldSchedule(ctx context.Context) bool {
	return j.settingsService.GetBoolSetting(ctx, "autoHealEnabled", false)
}
func (j *AutoHealJob) Schedule(ctx context.Context) string {
	schedule := cmp.Or(j.settingsService.GetStringSetting(ctx, "autoHealInterval", autoHealDefaultSchedule), autoHealDefaultSchedule)
	if _, err := scheduleutil.Parser().Parse(schedule); err != nil {
		return autoHealDefaultSchedule
	}
	return schedule
}
func healRunContext(ctx context.Context) context.Context {
	if _, ok := jobcontext.Run(ctx); !ok {
		return jobcontext.WithExecution(ctx, st.Run{ID: uuid.NewString(), JobID: AutoHealJobName}, nil)
	}
	return ctx
}
func (j *AutoHealJob) Run(ctx context.Context) (st.Outcome, error) {
	ctx = healRunContext(ctx)
	lease, admitted, err := j.admissionGate.TryAcquire(ctx, st.AdmissionKey{Scope: autoHealAdmissionScopeInternal})
	if err != nil {
		return st.Outcome{}, err
	}
	if !admitted {
		return st.Outcome{Status: st.Skipped}, nil
	}
	defer lease.Release(ctx)
	if j.store == nil || j.store.DB == nil {
		return st.Outcome{}, errors.New("auto-heal durable store unavailable")
	}
	cli, err := j.getDockerClientInternal(ctx)
	if err != nil {
		return st.Outcome{}, err
	}
	if err = j.recover(ctx, cli); err != nil {
		return st.Outcome{}, err
	}
	if !j.ShouldSchedule(ctx) {
		return st.Outcome{Status: st.Skipped}, nil
	}
	// Candidates fail closed on settings outside the typed contract; the run names the field once.
	if cfg, cfgErr := j.settingsService.AutoHealSettings(ctx); cfgErr == nil && cfg.Enabled {
		if problem, invalid := errors.AsType[*autohealDTO.Problem](autohealUpdate(cfg).Validate()); invalid {
			slog.WarnContext(ctx, "auto-heal settings outside the typed contract; host skipped", "code", problem.Code, "field", problem.Field)
		}
	}
	rows, err := j.ListContainers(ctx, cli)
	if err != nil {
		return st.Outcome{}, err
	}
	candidates := j.filterCandidatesInternal(rows, j.parseExcludedContainers(ctx), j.selfContainerIDInternal(ctx))
	plan := autoHealPlanInternal{Containers: candidates}
	frozen, err := json.Marshal(plan)
	if err != nil {
		return st.Outcome{}, err
	}
	if err = jobcontext.Progress(ctx, st.TargetOutcome{ResourceType: "heal_plan", ID: "auto-heal-plan", Status: st.Succeeded, RecoveryData: frozen}); err != nil {
		return st.Outcome{}, err
	}
	outcome := st.Outcome{Status: st.Succeeded}
	var mu sync.Mutex
	g, groupCtx := errgroup.WithContext(ctx)
	g.SetLimit(autoHealInspectConcurrency)
	for _, candidate := range candidates {
		g.Go(func() error {
			target, err := j.processCandidateInternal(groupCtx, cli, candidate, 0, 0, 0)
			mu.Lock()
			outcome.Targets = append(outcome.Targets, target)
			if err != nil {
				outcome.Status = st.Partial
			}
			mu.Unlock()
			return jobcontext.Progress(groupCtx, target)
		})
	}
	err = g.Wait()
	return outcome, err
}
func (j *AutoHealJob) recover(ctx context.Context, cli *client.Client) error {
	return j.store.Recover(ctx, func(ctx context.Context, id string) (autoheal.Baseline, error) {
		v, err := j.inspectContainerInternal(ctx, cli, id)
		if err != nil {
			return autoheal.Baseline{}, err
		}
		if v.State == nil || v.ID != id {
			return autoheal.Baseline{}, errors.New("container unavailable")
		}
		return autoheal.Baseline{StartedAt: v.State.StartedAt, RestartCount: v.RestartCount, Restarting: v.State.Restarting}, nil
	})
}
func targetForRestart(r *autoheal.Restart) st.TargetOutcome {
	status := st.Failed
	if r.Outcome == "restarted" {
		status = st.Succeeded
	}
	if r.Outcome == "limit-reached" {
		status = st.Skipped
	}
	return st.TargetOutcome{ResourceType: "container", ID: r.ContainerID, Status: status, Code: "autoheal." + r.Outcome}
}
func (j *AutoHealJob) processCandidateInternal(ctx context.Context, cli *client.Client, candidate container.Summary, _ int, _ time.Duration, _ int) (st.TargetOutcome, error) {
	target := st.TargetOutcome{ResourceType: "container", ID: candidate.ID, Status: st.Skipped}
	run, _ := jobcontext.Run(ctx)

	v, err := j.inspectContainerInternal(ctx, cli, candidate.ID)
	if err != nil {
		return target, err
	}
	// Recheck identity, running state, healthcheck and current configuration immediately before reservation.
	if v.ID != candidate.ID || v.State == nil || !v.State.Running || v.State.Restarting || v.State.Paused || v.State.Health == nil || v.State.Health.Status != container.Unhealthy || v.Config == nil || v.Config.Healthcheck == nil || len(v.Config.Healthcheck.Test) == 0 || v.Config.Healthcheck.Test[0] == "NONE" {
		return target, nil
	}
	cfg, err := j.settingsService.AutoHealSettings(ctx)
	if err != nil {
		return target, err
	}
	name := strings.TrimPrefix(v.Name, "/")
	if name == "" {
		name = docker.ContainerNameFromNames(candidate.Names)
	}
	stack := v.Config.Labels["com.docker.compose.project"]
	internal, _ := kit.ParseBool(v.Config.Labels[libarcane.InternalResourceLabel])
	if !cfg.Enabled || internal || slices.Contains(cfg.ExcludedContainers, name) || slices.Contains(cfg.ExcludedStacks, stack) || (j.selfContainerIDInternal(ctx) != "" && strings.HasPrefix(strings.ToLower(v.ID), j.selfID)) {
		return target, nil
	}
	if err := (autohealUpdate(cfg)).Validate(); err != nil {
		return target, err
	}
	if j.busy == nil {
		return target, errors.New("auto-heal ownership snapshot unavailable")
	}
	held, readErr := j.busy(ctx, v)
	if held || readErr != nil {
		target.Message = "skipped-busy"
		return target, nil
	}
	r, restart, err := j.store.Reserve(ctx, run.ID, v.ID, name, stack, autoheal.Baseline{StartedAt: v.State.StartedAt, RestartCount: v.RestartCount}, cfg)
	if err != nil {
		return target, err
	}
	if r == nil {
		return target, nil
	}
	if !restart {
		if err := j.store.Project(ctx, r.ID, ctx.Value(autoHealReplayKey{}) != true); err != nil {
			return target, err
		}
		return targetForRestart(r), nil
	}
	if j.afterReserve != nil {
		j.afterReserve()
	}
	frozen, _ := json.Marshal(autoHealBaselineInternal{StartedAt: v.State.StartedAt, RestartCount: v.RestartCount})
	target.Status = st.Running
	target.RecoveryData = frozen
	if err := jobcontext.Progress(ctx, target); err != nil {
		return target, err
	}
	restartErr := j.restartContainerInternal(ctx, cli, v.ID)
	outcome := "restarted"
	if restartErr != nil {
		outcome = "restart-failed"
	}
	if err := j.store.Settle(ctx, r.ID, outcome); err != nil {
		return target, err
	}
	r.Outcome = outcome
	if err := j.store.Project(ctx, r.ID, ctx.Value(autoHealReplayKey{}) != true); err != nil {
		return target, err
	}
	target = targetForRestart(r)
	if restartErr != nil {
		return target, errors.New("autoheal.restart-failed")
	}
	return target, nil
}
func autohealUpdate(s autohealDTO.Settings) autohealDTO.Update {
	return autohealDTO.Update{Enabled: s.Enabled, MaxRestarts: s.MaxRestarts, WindowMinutes: s.WindowMinutes, ExcludedStacks: s.ExcludedStacks, ExcludedContainers: s.ExcludedContainers}
}

// selfContainerIDInternal resolves and caches the ID (full 64-char or short
// prefix) of the container Arcane itself runs in. Returns "" when detection
// fails (e.g. binary running directly on the host), disabling the guard.
func (j *AutoHealJob) selfContainerIDInternal(ctx context.Context) string {
	j.selfIDOnce.Do(func() {
		detect := j.getSelfContainerID
		if detect == nil {
			detect = cgroup.CurrentContainerID
		}
		id, err := detect()
		if err != nil {
			slog.DebugContext(ctx, "auto-heal: could not determine own container ID; self-protection disabled", "error", err)
			return
		}
		j.selfID = strings.ToLower(strings.TrimSpace(id))
		slog.InfoContext(ctx, "auto-heal: detected own container; it will never be auto-restarted", "containerId", j.selfID)
	})
	return j.selfID
}

func (j *AutoHealJob) filterCandidatesInternal(containers []container.Summary, excludedContainers map[string]struct{}, selfID string) []container.Summary {
	candidates := make([]container.Summary, 0, len(containers))
	for _, c := range containers {
		// Never restart the container Arcane itself runs in: a slow or
		// mid-startup manager that trips its own healthcheck would otherwise
		// be restarted by its own auto-heal, in a loop. Prefix match because
		// hostname-based detection yields the short 12-char ID.
		if selfID != "" && strings.HasPrefix(strings.ToLower(c.ID), selfID) {
			continue
		}

		if internal, _ := kit.ParseBool(c.Labels[libarcane.InternalResourceLabel]); internal {
			continue
		}

		containerName := docker.ContainerNameFromNames(c.Names)
		if j.isExcluded(containerName, excludedContainers) {
			continue
		}

		candidates = append(candidates, c)
	}

	return candidates
}

func (j *AutoHealJob) parseExcludedContainers(ctx context.Context) map[string]struct{} {
	raw := j.settingsService.GetStringSetting(ctx, "autoHealExcludedContainers", "")
	excluded := make(map[string]struct{})
	if raw == "" {
		return excluded
	}
	for name := range strings.SplitSeq(raw, ",") {
		trimmed := strings.TrimSpace(name)
		if trimmed != "" {
			excluded[trimmed] = struct{}{}
		}
	}
	return excluded
}

func (j *AutoHealJob) isExcluded(name string, excluded map[string]struct{}) bool {
	_, ok := excluded[name]
	return ok
}

func (j *AutoHealJob) inspectContainerInternal(ctx context.Context, dockerClient *client.Client, containerID string) (container.InspectResponse, error) {
	if j.inspectContainer != nil {
		return j.inspectContainer(ctx, dockerClient, containerID)
	}

	inspect, err := compat.ContainerInspectWithCompatibility(ctx, dockerClient, containerID, client.ContainerInspectOptions{})
	if err != nil {
		return container.InspectResponse{}, err
	}

	return inspect.Container, nil
}

func (j *AutoHealJob) restartContainerInternal(ctx context.Context, dockerClient *client.Client, containerID string) error {
	if j.restartContainer != nil {
		return j.restartContainer(ctx, dockerClient, containerID)
	}

	_, err := dockerClient.ContainerRestart(ctx, containerID, client.ContainerRestartOptions{})
	return err
}

func (j *AutoHealJob) getDockerClientInternal(ctx context.Context) (*client.Client, error) {
	if j.getDockerClient != nil {
		return j.getDockerClient()
	}

	return j.dockerClientService.GetClient(ctx)
}

// autoHealListOptionsInternal filters unhealthy containers at the daemon, so
// the common all-healthy case returns an empty list and nothing is inspected.
func autoHealListOptionsInternal() client.ContainerListOptions {
	return client.ContainerListOptions{
		All:     false,
		Filters: make(client.Filters).Add("health", string(container.Unhealthy)),
	}
}

func (j *AutoHealJob) ListContainers(ctx context.Context, dockerClient *client.Client) ([]container.Summary, error) {
	if j.listContainers != nil {
		return j.listContainers(ctx, dockerClient)
	}

	containerList, err := dockerClient.ContainerList(ctx, autoHealListOptionsInternal())
	if err != nil {
		return nil, err
	}

	return containerList.Items, nil
}

type autoHealReplayKey struct{}

func (j *AutoHealJob) Reconcile(ctx context.Context, previous st.Run) (st.Outcome, error) {
	ctx = context.WithValue(ctx, autoHealReplayKey{}, true)
	ctx = jobcontext.WithExecution(ctx, previous, nil)
	lease, admitted, err := j.admissionGate.TryAcquire(ctx, st.AdmissionKey{Scope: autoHealAdmissionScopeInternal})
	if err != nil {
		return st.Outcome{}, err
	}
	if !admitted {
		return st.Outcome{Status: st.Failed}, nil
	}
	defer lease.Release(ctx)
	if j.store == nil {
		return st.Outcome{}, errors.New("auto-heal durable store unavailable")
	}
	cli, err := j.getDockerClientInternal(ctx)
	if err != nil {
		return st.Outcome{}, err
	}
	if err = j.recover(ctx, cli); err != nil {
		return st.Outcome{}, err
	}
	for i, t := range previous.Outcome.Targets {
		if t.ResourceType != "container" {
			continue
		}
		if r, err := j.store.Lookup(ctx, previous.ID, t.ID); err != nil {
			return st.Outcome{}, err
		} else if r != nil {
			previous.Outcome.Targets[i] = targetForRestart(r)
		}
	}
	var plan autoHealPlanInternal
	for _, t := range previous.Outcome.Targets {
		if t.ID == "auto-heal-plan" {
			if err := json.Unmarshal(t.RecoveryData, &plan); err != nil {
				return st.Outcome{}, err
			}
			break
		}
	}
	return j.resumeHealingPlanInternal(ctx, cli, plan, previous)
}
func (j *AutoHealJob) resumeHealingPlanInternal(ctx context.Context, cli *client.Client, plan autoHealPlanInternal, previous st.Run) (st.Outcome, error) {
	outcome := st.Outcome{Status: st.Succeeded, Targets: previous.Outcome.Targets}
	for _, candidate := range plan.Containers {
		attempted := false
		for _, t := range outcome.Targets {
			if t.ID == candidate.ID {
				attempted = true
				break
			}
		}
		if attempted {
			continue
		}
		// Reservation may have committed before the scheduler target checkpoint.
		if r, err := j.store.Lookup(ctx, previous.ID, candidate.ID); err != nil {
			return st.Outcome{}, err
		} else if r != nil {
			target := targetForRestart(r)
			outcome.Targets = append(outcome.Targets, target)
			if err := jobcontext.Progress(ctx, target); err != nil {
				return st.Outcome{}, err
			}
			continue
		}
		target, err := j.processCandidateInternal(ctx, cli, candidate, 0, 0, 0)
		if err != nil {
			outcome.Status = st.Partial
		}
		outcome.Targets = append(outcome.Targets, target)
		if err := jobcontext.Progress(ctx, target); err != nil {
			return st.Outcome{}, err
		}
	}
	for _, target := range outcome.Targets {
		if target.Status == st.Failed {
			outcome.Status = st.Partial
		}
	}
	return outcome, nil
}
