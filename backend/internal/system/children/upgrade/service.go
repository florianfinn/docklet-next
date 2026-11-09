// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; replace Arcane upgrader and fleet jobs with correlated Compose self-updates; fallback container search uses the Docklet system label (#235).

package upgrade

import (
	"context"
	"errors"
	"net/http"
	"sync"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	dockerInternal "github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/kv"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/system"
	product "github.com/florianfinn/docklet-next/types/version"
	"github.com/google/uuid"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"go.getarcane.app/sys/cgroup"
	"gorm.io/gorm"
)

type Service struct {
	runningVersion func() string

	db           *database.DB
	docker       *dockerInternal.DockerClientService
	versions     *version.VersionService
	cfg          *config.Config
	environments *environment.EnvironmentService
	store        *core.Store
	ctx          context.Context
	cancel       context.CancelFunc
	mu           sync.Mutex
	workers      sync.WaitGroup
	active       map[string]bool
	selfID       func() (string, error)
	resolve      func(context.Context, string, string, http.RoundTripper) (string, error)
	backupBusy   func(context.Context) (bool, error)
	transport    http.RoundTripper
	data         string
}

func NewService(db *database.DB, docker *dockerInternal.DockerClientService, versions *version.VersionService, cfg *config.Config) *Service {
	return &Service{db: db, docker: docker, versions: versions, cfg: cfg, store: core.NewStore(db), runningVersion: func() string { return config.Version }, selfID: cgroup.CurrentContainerID, resolve: core.ResolveTarget, active: map[string]bool{}, data: "/app/data"}
}
func (s *Service) SetBackupBusy(check func(context.Context) (bool, error)) { s.backupBusy = check }
func (s *Service) SetEnvironments(environments *environment.EnvironmentService) {
	s.environments = environments
}
func (s *Service) role() string {
	if s.cfg != nil && (s.cfg.AgentMode || s.cfg.EdgeAgent) {
		return "agent"
	}
	return "hub"
}
func (s *Service) endpoint() string {
	if s.cfg != nil && s.cfg.DockerHost != "" {
		return s.cfg.DockerHost
	}
	return "unix:///var/run/docker.sock"
}
func repository(role string) string { return config.ReleaseRepository + "-" + role }

// Start restores observations only; it never resends a remote action or helper.
// The application calls this without waiting for any update's final result.
func (s *Service) Start(ctx context.Context) error {
	s.ctx, s.cancel = context.WithCancel(ctx)
	s.launch("local-observer", func(ctx context.Context) { s.observer().Run(ctx) })
	entries, err := kv.NewKVService(s.db).ListByPrefix(ctx, "selfupdate.remote.")
	if err != nil {
		return err
	}
	for _, entry := range entries {
		s.resumeRemote(entry.Key)
	}
	return nil
}
func (s *Service) Stop() {
	s.mu.Lock()
	if s.cancel != nil {
		s.cancel()
	}
	s.mu.Unlock()
	s.workers.Wait()
}
func (s *Service) launch(key string, work func(context.Context)) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ctx == nil || s.ctx.Err() != nil || s.active[key] {
		return
	}
	s.active[key] = true
	s.workers.Add(1)
	go func() {
		defer s.workers.Done()
		defer func() { s.mu.Lock(); delete(s.active, key); s.mu.Unlock() }()
		work(s.ctx)
	}()
}
func (s *Service) Check(ctx context.Context, id string) (dto.SelfUpdateCheck, error) {
	if id != "0" {
		return s.checkRemote(ctx, id)
	}
	out := dto.SelfUpdateCheck{Version: s.runningVersion()}
	installation, _, err := s.inspect(ctx)
	host := core.Installation{Role: s.role(), Version: s.runningVersion(), Repository: repository(s.role())}
	if err == nil && installation.Config != nil {
		host.CurrentReference = installation.Config.Image
		replay, replayErr := core.ReplayFromLabels(installation.Config.Labels, s.role(), installation.Config.Image)
		host.Compose = replayErr == nil
		if replayErr == nil {
			_, mountErr := core.HelperMounts(installation, s.endpoint(), s.data, replay)
			host.Replayable = mountErr == nil
		}
	}
	op, readErr := s.store.Read(ctx)
	if readErr != nil {
		return out, readErr
	}
	if op != nil {
		running, _ := s.helperRunning(ctx, *op)
		view := op.DTO(running)
		out.Operation = &view
		host.Busy = op.Phase != "settled"
	}
	feed := core.Feed{}
	if s.role() == "hub" {
		feed = s.feed(ctx, "hub", s.runningVersion())
	}
	out.Eligibility = core.Eligibility(host, feed, "")
	if installation.Config != nil {
		if replay, e := core.ReplayFromLabels(installation.Config.Labels, s.role(), installation.Config.Image); e == nil {
			out.Eligibility.Manual = core.ManualCommands(replay, op)
		}
	}
	if err != nil {
		out.Eligibility.State = "unavailable"
		out.Eligibility.Code = "agent.unavailable"
	}
	return out, nil
}
func (s *Service) feed(ctx context.Context, role, current string) core.Feed {
	if s.versions == nil {
		return core.Feed{Code: "update.check-unavailable"}
	}
	newest, ok, err := s.versions.NewestRelease(ctx)
	if err != nil {
		return core.Feed{Code: feedCode(err)}
	}
	if !ok {
		return core.Feed{}
	}
	chosen := newest
	if role == "agent" {
		limit, valid := product.ParseProduct(s.runningVersion())
		next, _ := product.ParseProduct(newest.Version)
		if !valid {
			return core.Feed{Code: "update.system-version-unknown"}
		}
		if next.Compare(limit) > 0 {
			capped, found, e := s.versions.ReleaseAtMost(ctx, s.runningVersion())
			if e != nil {
				return core.Feed{Code: feedCode(e)}
			}
			running, valid := product.ParseProduct(current)
			capVersion, _ := product.ParseProduct(capped.Version)
			if !found || !valid || capVersion.Compare(running) <= 0 {
				return core.Feed{Candidate: &dto.SelfUpdateCandidate{Version: newest.Version}}
			}
			chosen = capped
		}
	}
	return core.Feed{Candidate: &dto.SelfUpdateCandidate{Version: chosen.Version}}
}
func feedCode(err error) string {
	var e *imageDTO.OperationError
	if errors.As(err, &e) && e.Problem.Code == "feature-disabled" {
		return "feature-disabled"
	}
	return "update.check-unavailable"
}
func (s *Service) inspect(ctx context.Context) (container.InspectResponse, *client.Client, error) {
	engine, err := s.docker.GetClient(ctx)
	if err != nil {
		return container.InspectResponse{}, nil, err
	}
	id, err := s.selfID()
	if err != nil {
		return container.InspectResponse{}, engine, err
	}
	response, err := engine.ContainerInspect(ctx, id, client.ContainerInspectOptions{})
	if err == nil && (response.Container.Config == nil || response.Container.Config.Labels["io.docklet.helper"] != "") {
		err = core.Refuse(core.CodeLayout)
	}
	return response.Container, engine, err
}

func (s *Service) reserveRun(ctx context.Context, store *core.Store, op core.Operation, userID, username string) (core.Operation, bool, error) {
	return store.Reserve(ctx, op, func(tx *gorm.DB, current *core.Operation) error {
		if err := activity.CheckHostUpdateAdmissionTx(tx, current.EnvironmentID); err != nil {
			return err
		}
		if current.EnvironmentID != "0" {
			if err := s.boundTx(tx, *current); err != nil {
				return err
			}
		}
		if s.role() == "agent" {
			return nil
		}
		now := time.Now().UTC()
		kind := activityDTO.TypeHubUpdate
		if current.EnvironmentID != "0" {
			kind = activityDTO.TypeAgentUpdate
		}
		run := activity.Activity{BaseModel: database.BaseModel{ID: uuid.NewString(), CreatedAt: now}, EnvironmentID: current.EnvironmentID, Type: kind, Status: activityDTO.StatusRunning, StartedAt: now, StartedByUserID: new(userID), StartedByUsername: new(username), MessageCode: string(activityDTO.CodeRunStarted), ResourceType: new("system"), ResourceID: new(current.EnvironmentID), Metadata: database.JSON{"opId": current.OpID, "targetVersion": current.TargetVersion, "targetReference": current.TargetRef}}
		if err := tx.Create(&run).Error; err != nil {
			return err
		}
		current.RunID = run.ID
		return nil
	})
}
func (s *Service) settleRun(tx *gorm.DB, op core.Operation) error {
	if op.RunID == "" {
		return nil
	}
	status := activityDTO.StatusFailed
	code := "activity.failed"
	if op.Outcome == "healthy" {
		status = activityDTO.StatusSuccess
		code = "activity.success"
	}
	if op.Code != "" {
		code = op.Code
	}
	now := time.Now().UTC()
	return tx.Model(&activity.Activity{}).Where("id=? AND status IN ?", op.RunID, []activityDTO.Status{activityDTO.StatusRunning, activityDTO.StatusQueued}).Updates(map[string]any{"status": status, "message_code": code, "ended_at": now, "updated_at": now}).Error
}
func response(op core.Operation) dto.SelfUpdateResponse {
	return dto.SelfUpdateResponse{OpID: op.OpID, Accepted: true, Phase: op.Phase, Code: op.Code, RunID: op.RunID}
}
