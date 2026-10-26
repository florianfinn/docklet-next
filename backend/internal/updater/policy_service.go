// Source: docklet-next/docs/design/update-policies.md
// Revision: cdfe93c3dd2557cb21586ae542ebc05cae9bc0af
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Hub-only policy configuration, trusted execution ports and actor revalidation.

package updater

import (
	"context"
	"encoding/json"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"net/http"
	"net/url"
	"strings"
	"time"
)

type imagePolicyPorts struct {
	hints         func(context.Context, string) (dto.SystemImageHints, error)
	scheduleActor func(context.Context, string, string, string) error
	host          func(context.Context, string) (string, error)
	actor         func(context.Context, string, string, string, bool) (context.Context, error)
	baseline      func(context.Context, string, string) (dto.BaselineStatus, error)
	check         func(context.Context, string, string) (*projectDTO.UpdateInfo, error)
	admit         func(context.Context, string, string, string, dto.LifecycleRequest, dto.PolicyBinding) (activity.RuntimeAdmissionResult, error)
	wait          func(context.Context, string, string) (activityDTO.RuntimeSnapshot, error)
	snapshot      func(context.Context, string, string) (activityDTO.RuntimeSnapshot, bool, error)
	notify        func(context.Context, string, string, dto.UpdateCandidate) (int, error)
}
type imagePolicyService struct {
	startedAt  time.Time
	store      policy.Store
	ports      imagePolicyPorts
	reschedule func(context.Context) error
}

func IsImagePolicyJob(id string) bool          { return id == "image-polling" || id == "auto-update" }
func ImagePolicyJobKey(env, job string) string { return "image-policy:" + env + ":" + job }
func ParseImagePolicyJob(id string) (string, string, bool) {
	parts := strings.Split(id, ":")
	if len(parts) != 3 || parts[0] != "image-policy" || parts[1] == "" || !IsImagePolicyJob(parts[2]) {
		return "", "", false
	}
	return parts[1], parts[2], true
}
func (s *UpdaterService) hubPolicies() (*imagePolicyService, error) {
	if s == nil || (s.config != nil && (s.config.AgentMode || s.config.EdgeAgent)) {
		return nil, imageDTO.Failure("update.schedule-hub-owned", 409)
	}
	if s.policies == nil {
		return nil, imageDTO.Failure("update.persistence-failed", 500)
	}
	return s.policies, nil
}
func (s *UpdaterService) ConfigureImagePolicies(envs *environment.EnvironmentService, reschedule func(context.Context) error) {
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return
	}
	p := &imagePolicyService{store: policy.Store{DB: s.deps.DB.DB}, reschedule: reschedule, startedAt: time.Now().UTC()}
	p.ports.host = func(ctx context.Context, env string) (string, error) {
		if env == "0" {
			return s.deps.Settings.GetSettingsConfig().InstanceID.Value, nil
		}
		if envs == nil {
			return "", imageDTO.Failure("update.target-not-found", 404)
		}
		_, err := envs.GetEnvironmentByID(ctx, env)
		return env, err
	}
	p.ports.actor = s.imagePolicyActor
	p.ports.scheduleActor = func(ctx context.Context, env, actor, key string) error {
		if s.roles == nil {
			return huma.Error403Forbidden("Schedule actor unavailable")
		}
		grants, e := s.roles.ResolveExecutionPermissions(ctx, actor, key)
		if e != nil || !grants.Allows(authz.PermJobsManage, env) {
			return huma.Error403Forbidden("Schedule grants required")
		}
		return nil
	}
	p.ports.baseline = s.imagePolicyBaseline(envs)
	p.ports.check = func(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
		if env == "0" {
			return s.CheckProjectUpdates(ctx, target)
		}
		var out base.ApiResponse[*projectDTO.UpdateInfo]
		err := envs.ProxyJSONRequest(ctx, env, http.MethodPost, "/api/environments/0/updater/projects/"+url.PathEscape(target)+"/check", nil, &out)
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	p.ports.admit = s.deps.Activity.AdmitPolicyUpdate
	p.ports.wait = s.deps.Activity.WaitPolicyUpdate
	p.ports.snapshot = s.deps.Activity.PolicyUpdateSnapshot
	if s.deps.Notifications != nil {
		p.ports.notify = s.deps.Notifications.NotifyImagePolicyCandidate
	}
	s.policies = p
}
func (s *UpdaterService) imagePolicyActor(ctx context.Context, env, actor, key string, apply bool) (context.Context, error) {
	if s.roles == nil || actor == "" {
		return ctx, huma.Error403Forbidden("Policy actor unavailable")
	}
	ps, err := s.roles.ResolveExecutionPermissions(ctx, actor, key)
	if err != nil {
		return ctx, huma.Error403Forbidden("Policy actor unavailable")
	}
	required := imagePolicyGrants(apply)
	for _, right := range required {
		if !ps.Allows(right, env) {
			return ctx, huma.Error403Forbidden("Policy actor grants required")
		}
	}
	var who struct {
		ID          string
		Username    string
		DisplayName *string
	}
	if err = s.deps.DB.WithContext(ctx).Table("users").Select("id,username,display_name").Where("id = ?", actor).Take(&who).Error; err != nil {
		return ctx, huma.Error403Forbidden("Policy actor unavailable")
	}
	ctx = context.WithValue(ctx, middleware.ContextKeyUserPermissions, ps)
	ctx = context.WithValue(ctx, middleware.ContextKeyUserID, who.ID)
	ctx = context.WithValue(ctx, middleware.ContextKeyApiKeyID, key)
	identity := &user.Actor{ID: who.ID, Username: who.Username, DisplayName: who.DisplayName}
	return context.WithValue(ctx, user.CurrentUserContextKey{}, identity), nil
}
func (s *UpdaterService) GetImagePolicy(ctx context.Context, env, target string) (dto.AutoUpdatePolicy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	row, err := p.store.Policy(ctx, env, reg, target)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	if row.Revision == 0 {
		live, e := p.ports.baseline(ctx, env, target)
		if e != nil || !live.Valid() {
			return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.check-unavailable", 503)
		}
		row.TargetIdentity = live.TargetIdentity
	}
	out := dto.AutoUpdatePolicy{Mode: row.Mode, Revision: row.Revision, EnvironmentID: env, RegistrationID: reg, TargetKind: "project", TargetID: target, TargetIdentity: row.TargetIdentity, JobID: "auto-update", CheckedAt: row.CheckedAt, Code: row.Code, ApplyCheckedAt: row.ApplyCheckedAt, ApplyCode: row.ApplyCode, BookkeepingAt: row.BookkeepingAt, BookkeepingCode: row.BookkeepingCode, Candidates: []dto.UpdateCandidate{}}
	if row.Mode == "notify" {
		out.JobID = "image-polling"
	}
	out.Stale = row.CheckedAt == nil || row.Code != "" || row.Mode == "off"
	if env != "0" {
		var host struct {
			Status  string
			Enabled bool
		}
		if s.deps.DB.WithContext(ctx).Table("environments").Select("status,enabled").Where("id = ?", env).Take(&host).Error != nil || !host.Enabled || host.Status != "online" {
			out.Stale = true
		}
	}
	if row.Candidates != "" && json.Unmarshal([]byte(row.Candidates), &out.Candidates) != nil {
		return out, imageDTO.Failure("update.persistence-failed", 500)
	}
	return out, nil
}
func (s *UpdaterService) SaveImagePolicy(ctx context.Context, env, target string, in dto.UpdatePolicyInput, actor, key string) (dto.AutoUpdatePolicy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	if in.Mode != "off" && in.Mode != "notify" && in.Mode != "apply" {
		return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.invalid-policy", 400)
	}
	if _, err = p.ports.actor(ctx, env, actor, key, in.Mode == "apply"); err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	row, err := p.store.Policy(ctx, env, reg, target)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	// Disabling a known policy needs no executor observation, including offline
	// hosts. Enabling/rebinding still proves the current target identity afresh.
	if in.Mode != "off" || row.Revision == 0 {
		baseline, e := p.ports.baseline(ctx, env, target)
		if e != nil {
			return dto.AutoUpdatePolicy{}, e
		}
		if !baseline.Valid() {
			return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.check-unavailable", 503)
		}
		row.TargetIdentity = baseline.TargetIdentity
	}
	row.Mode = in.Mode
	row.ActorID = actor
	row.ActorKeyID = key
	row.CheckedAt = nil
	row.Code = ""
	row.ApplyCheckedAt = nil
	row.ApplyCode = ""
	row.BookkeepingAt = nil
	row.BookkeepingCode = ""
	if err = p.store.SavePolicy(ctx, row, in.ExpectedRevision); err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	return s.GetImagePolicy(ctx, env, target)
}
func (s *UpdaterService) ImagePolicySchedule(ctx context.Context, env string) (policy.HostSchedule, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return policy.HostSchedule{}, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return policy.HostSchedule{}, err
	}
	return p.store.Schedule(ctx, env, reg)
}
func (s *UpdaterService) SaveImagePolicySchedule(ctx context.Context, env string, in dto.UpdateScheduleInput, actor, key string) error {
	p, err := s.hubPolicies()
	if err != nil {
		return err
	}
	// A schedule controls trigger authority; each target retains its policy actor.
	if s.roles == nil {
		return huma.Error403Forbidden("Schedule actor unavailable")
	}
	ps, err := s.roles.ResolveExecutionPermissions(ctx, actor, key)
	if err != nil || !ps.Allows(authz.PermJobsManage, env) {
		return huma.Error403Forbidden("Schedule grants required")
	}
	check, e := schedule.NormalizeSixField(in.PollingInterval, "polling")
	if e != nil {
		return imageDTO.Failure("update.invalid-policy", 400)
	}
	apply, e := schedule.NormalizeSixField(in.AutoUpdateInterval, "auto-update")
	if e != nil {
		return imageDTO.Failure("update.invalid-policy", 400)
	}
	row, err := s.ImagePolicySchedule(ctx, env)
	if err != nil {
		return err
	}
	row.PollingInterval = check
	row.AutoUpdateInterval = apply
	row.PollingEnabled = in.PollingEnabled
	row.AutoUpdateEnabled = in.AutoUpdateEnabled
	row.ActorID = actor
	row.ActorKeyID = key
	if err = p.store.SaveSchedule(ctx, row, in.ExpectedRevision); err != nil {
		return err
	}
	if p.reschedule != nil {
		return p.reschedule(ctx)
	}
	return nil
}
func (s *UpdaterService) ImagePolicyEnabled(ctx context.Context, row policy.HostSchedule, job string) bool {
	if row.EnvironmentID != "0" {
		var host struct{ Enabled bool }
		if s.deps.DB.WithContext(ctx).Table("environments").Select("enabled").Where("id = ?", row.EnvironmentID).Take(&host).Error != nil || !host.Enabled {
			return false
		}
	}
	if row.ActorID == "" {
		return false
	}
	polling := s.deps.Settings.GetBoolSetting(ctx, "pollingEnabled", true)
	if job == "image-polling" {
		return polling && row.PollingEnabled
	}
	return polling && s.deps.Settings.GetBoolSetting(ctx, "autoUpdate", false) && row.AutoUpdateEnabled
}
func (s *UpdaterService) RefreshImagePolicySchedules(ctx context.Context) error {
	p, err := s.hubPolicies()
	if err != nil {
		return err
	}
	if p.reschedule != nil {
		return p.reschedule(ctx)
	}
	return nil
}
func policyCode(err error, fallback string) string {
	var problem *imageDTO.OperationError
	if errors.As(err, &problem) {
		if imageDTO.ValidReason(problem.Problem.Code) {
			return problem.Problem.Code
		}
		return fallback
	}
	var gate *activity.GateConflict
	if errors.As(err, &gate) {
		if imageDTO.ValidReason(gate.Code) {
			return gate.Code
		}
		return fallback
	}
	var status huma.StatusError
	if errors.As(err, &status) && (status.GetStatus() == 403 || status.GetStatus() == 401) {
		return "update.permission-denied"
	}
	return fallback
}
func policyNow() *time.Time {
	now := time.Now().UTC()
	return &now
}

func (s *UpdaterService) ImagePolicyHosts(ctx context.Context) ([]string, error) {
	if _, err := s.hubPolicies(); err != nil {
		return nil, err
	}
	ids := []string{}
	if err := s.deps.DB.WithContext(ctx).Table("environments").Order("id").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := []string{"0"}
	for _, id := range ids {
		if id != "0" {
			out = append(out, id)
		}
	}
	return out, nil
}

func imagePolicyGrants(apply bool) []string {
	required := []string{authz.PermProjectsRead, authz.PermImageUpdatesCheck, authz.PermImageUpdatesRead}
	if apply {
		required = append(required, authz.PermJobsManage, authz.PermProjectsDeploy, authz.PermProjectsUpdate, authz.PermImagesPull, authz.PermActivitiesRead)
	}
	return required
}

type imagePolicyProxy interface {
	ProxyJSONRequest(context.Context, string, string, string, []byte, any) error
}

func (s *UpdaterService) imagePolicyBaseline(envs imagePolicyProxy) func(context.Context, string, string) (dto.BaselineStatus, error) {
	return func(ctx context.Context, env, target string) (dto.BaselineStatus, error) {
		if env == "0" {
			return s.deps.Projects.ImageBaselineStatus(ctx, target, s.deps.Projects.LifecycleRegistration(ctx))
		}
		var out base.ApiResponse[dto.BaselineStatus]
		err := envs.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/updater/projects/"+url.PathEscape(target)+"/baseline", nil, &out)
		if err != nil {
			return dto.BaselineStatus{}, err
		}
		return checkedBaselineResponse(out)
	}
}
