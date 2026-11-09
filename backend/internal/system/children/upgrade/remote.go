// Source: getarcaneapp/arcane/backend/internal/system/children/upgrade/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; replace Arcane upgrader and fleet jobs with correlated Compose self-updates; fallback container search uses the Docklet system label (#235).

package upgrade

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	core "github.com/florianfinn/docklet-next/backend/internal/selfupdate"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/edge"
	"github.com/florianfinn/docklet-next/backend/pkg/remenv"
	dto "github.com/florianfinn/docklet-next/types/system"
	product "github.com/florianfinn/docklet-next/types/version"
	"github.com/google/uuid"
	"gorm.io/gorm"
)

func (s *Service) remoteStore(id string) *core.Store {
	return &core.Store{DB: s.db, Key: "selfupdate.remote." + id}
}
func (s *Service) boundTx(tx *gorm.DB, op core.Operation) error {
	var env environment.Environment
	if err := tx.First(&env, "id=?", op.EnvironmentID).Error; err != nil {
		return err
	}
	if env.CredentialRevision != op.CredentialRevision {
		return core.Refuse(core.CodeUnknown)
	}
	return nil
}
func (s *Service) remoteEnvironment(ctx context.Context, id string) (environment.Environment, error) {
	if s.environments == nil {
		return environment.Environment{}, core.Refuse("agent.unavailable")
	}
	env, err := s.environments.GetEnvironmentByID(ctx, id)
	if err != nil {
		return environment.Environment{}, err
	}
	if !env.Enabled || env.AccessToken == nil {
		return environment.Environment{}, core.Refuse("agent.unavailable")
	}
	return *env, nil
}
func (s *Service) readRemote(ctx context.Context, env environment.Environment) (dto.SelfUpdateCheck, error) {
	if env.IsEdge {
		edge.TouchTunnelDemand(env.ID, 10*time.Second)
	}
	bounded, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	var out dto.SelfUpdateCheck
	err := s.environments.ProxyJSONRequestForEnvironment(bounded, env, http.MethodGet, "/api/environments/0/system/upgrade/check", nil, &out)
	if err == nil {
		// Token/endpoint rotations and delete+re-register invalidate an in-flight
		// reply. A fresh Edge session under the same registration remains valid.
		current, e := s.remoteEnvironment(ctx, env.ID)
		if e != nil || current.CredentialRevision != env.CredentialRevision {
			err = core.Refuse(core.CodeUnknown)
		}
	}
	return out, err
}
func (s *Service) checkRemote(ctx context.Context, id string) (dto.SelfUpdateCheck, error) {
	env, err := s.remoteEnvironment(ctx, id)
	if err != nil {
		return dto.SelfUpdateCheck{}, err
	}
	out, err := s.readRemote(ctx, env)
	if err != nil {
		return out, err
	}
	feed := s.feed(ctx, "agent", out.Version)
	// Local replay/custom-image exclusions survive the hub's feed selection.
	if out.Eligibility.State != "not-eligible" && out.Eligibility.State != "unavailable" {
		manual := out.Eligibility.Manual
		out.Eligibility = core.Eligibility(core.Installation{Role: "agent", Version: out.Version, CurrentReference: repository("agent") + ":" + out.Version, Repository: repository("agent"), Compose: true, Replayable: true}, feed, s.runningVersion())
		out.Eligibility.Manual = manual
	}
	op, err := s.remoteStore(id).Read(ctx)
	if err != nil {
		return out, err
	}
	if op != nil {
		view := op.DTO(out.Operation != nil && out.Operation.HelperRunning)
		out.Operation = &view
		if op.Phase != "settled" {
			out.Eligibility.State = "not-eligible"
			out.Eligibility.Code = "host.busy"
		}
	}
	return out, nil
}
func (s *Service) startRemote(ctx context.Context, id string, input dto.SelfUpdateEnvelope, userID, username string) (dto.SelfUpdateResponse, error) {
	if s.role() != "hub" {
		return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
	}
	if input.OpID != "" || input.TargetReference != "" || input.HubVersion != "" {
		return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
	}
	check, err := s.checkRemote(ctx, id)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	if check.Eligibility.State != "available" {
		code := check.Eligibility.Code
		if code == "" {
			code = "update.invalid-target"
		}
		return dto.SelfUpdateResponse{}, core.Refuse(code)
	}
	candidate := check.Eligibility.Candidate
	if input.TargetVersion != "" && input.TargetVersion != candidate.Version {
		return dto.SelfUpdateResponse{}, core.Refuse("update.invalid-target")
	}
	target, ok := product.ParseProduct(candidate.Version)
	hub, hubOK := product.ParseProduct(s.runningVersion())
	if !ok || !hubOK || target.Compare(hub) > 0 {
		return dto.SelfUpdateResponse{}, core.Refuse(core.CodeHub)
	}
	ref, err := s.resolve(ctx, repository("agent"), candidate.Version, s.transport)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	env, err := s.remoteEnvironment(ctx, id)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	op := core.Operation{OpID: uuid.NewString(), Role: "agent", EnvironmentID: id, CredentialRevision: env.CredentialRevision, TargetRef: ref, TargetVersion: candidate.Version, FromVersion: check.Version, Deadline: time.Now().UTC().Add(core.OperationTimeout)}
	store := s.remoteStore(id)
	op, _, err = s.reserveRun(ctx, store, op, userID, username)
	if err != nil {
		return dto.SelfUpdateResponse{}, err
	}
	s.launch(store.Key, func(ctx context.Context) {
		changed, e := store.Change(ctx, op.OpID, func(tx *gorm.DB, current *core.Operation) error {
			if err := s.boundTx(tx, *current); err != nil {
				return err
			}
			current.Phase = "helper-started"
			return nil
		})
		if e == nil && changed {
			// The durable write precedes the only POST. Lost responses and restarts
			// enter observation, never a new operation or a replay of this request.
			envelope := dto.SelfUpdateEnvelope{OpID: op.OpID, TargetVersion: op.TargetVersion, TargetReference: op.TargetRef, HubVersion: s.runningVersion()}
			body, _ := json.Marshal(envelope)
			var accepted dto.SelfUpdateResponse
			sendCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			sendErr := s.environments.ProxyJSONRequestForEnvironment(sendCtx, env, http.MethodPost, "/api/environments/0/system/upgrade", body, &accepted)
			cancel()
			if code, refused := agentRefusal(sendErr); refused {
				// The agent refuses only before or at its own reservation, so a
				// decoded 4xx problem is definitive and nothing has run yet.
				_ = s.finishRemote(ctx, store, op, "failed-pre-effect", code)
				return
			}
		}
		s.observeRemote(ctx, store)
	})
	return response(op), nil
}

// agentRefusal reports a decoded HTTP 4xx problem from the agent and its
// stable code. Transport errors, 5xx and timeouts are not refusals: the
// request may have been executed, so the hub keeps observing.
func agentRefusal(err error) (string, bool) {
	var status *remenv.StatusError
	if !errors.As(err, &status) || status.StatusCode < 400 || status.StatusCode >= 500 {
		return "", false
	}
	// 408 is timeout semantics, and only a decoded agent problem (its own status
	// or a known code) proves a refusal; an intermediary body could replace an
	// accepted reply, so those stay in observation.
	var problem struct {
		Status int    `json:"status"`
		Code   string `json:"code"`
		Detail string `json:"detail"`
	}
	if status.StatusCode == http.StatusRequestTimeout || json.Unmarshal(status.Body, &problem) != nil {
		return "", false
	}
	for _, code := range []string{problem.Code, problem.Detail} {
		if refusalCodes[code] {
			return code, true
		}
	}
	if problem.Status == status.StatusCode {
		return core.CodeFailed, true
	}
	return "", false
}

var refusalCodes = map[string]bool{
	"host.busy": true, "update.invalid-target": true, "update.check-unavailable": true, "update.system-version-unknown": true, "feature-disabled": true,
	core.CodeNotCompose: true, core.CodeLayout: true, core.CodeCustom: true, core.CodeHub: true,
}

func (s *Service) resumeRemote(key string) {
	store := &core.Store{DB: s.db, Key: key}
	s.launch(key, func(ctx context.Context) { s.observeRemote(ctx, store) })
}
func (s *Service) finishRemote(ctx context.Context, store *core.Store, op core.Operation, outcome, code string) error {
	_, err := store.Change(ctx, op.OpID, func(tx *gorm.DB, current *core.Operation) error {
		if err := s.boundTx(tx, *current); err != nil {
			outcome = "unknown"
			code = core.CodeUnknown
		}
		current.Phase = "settled"
		current.Outcome = outcome
		current.Code = code
		return s.settleRun(tx, *current)
	})
	return err
}
func (s *Service) advanceRemote(ctx context.Context, store *core.Store) (bool, error) {
	op, err := store.Read(ctx)
	if err != nil || op == nil {
		return true, err
	}
	if op.Phase == "settled" {
		return true, nil
	}
	env, err := s.remoteEnvironment(ctx, op.EnvironmentID)
	if err == nil && env.CredentialRevision != op.CredentialRevision {
		return true, s.finishRemote(ctx, store, *op, "unknown", core.CodeUnknown)
	}
	var reply dto.SelfUpdateCheck
	if err == nil {
		reply, err = s.readRemote(ctx, env)
	}
	if err == nil && reply.Operation != nil && reply.Operation.OpID == op.OpID && reply.Operation.Phase == "settled" {
		outcome, code := reply.Operation.Outcome, reply.Operation.Code
		if outcome == "healthy" && reply.Version == op.TargetVersion {
			return true, s.finishRemote(ctx, store, *op, "healthy", "")
		}
		if outcome == "failed-health" || outcome == "failed-pre-effect" {
			return true, s.finishRemote(ctx, store, *op, outcome, core.CodeFailed)
		}
		if outcome == "unknown" {
			return true, s.finishRemote(ctx, store, *op, "unknown", core.CodeUnknown)
		}
		_ = code
	}
	now := time.Now()
	if !now.Before(op.Deadline) {
		helper := err == nil && reply.Operation != nil && reply.Operation.OpID == op.OpID && reply.Operation.HelperRunning
		if !helper || !now.Before(op.Deadline.Add(core.OperationTimeout)) {
			return true, s.finishRemote(ctx, store, *op, "unknown", core.CodeUnknown)
		}
	}
	if errors.Is(err, context.Canceled) {
		return false, err
	}
	return false, nil
}
func (s *Service) observeRemote(ctx context.Context, store *core.Store) {
	for ctx.Err() == nil {
		done, err := s.advanceRemote(ctx, store)
		if done && err == nil {
			return
		}
		timer := time.NewTimer(2 * time.Second)
		select {
		case <-ctx.Done():
			timer.Stop()
			return
		case <-timer.C:
		}
	}
}
