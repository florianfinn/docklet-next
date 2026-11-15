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
	containerService "github.com/florianfinn/docklet-next/backend/internal/container"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	"github.com/florianfinn/docklet-next/types/base"
	environmentDTO "github.com/florianfinn/docklet-next/types/environment"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/florianfinn/docklet-next/types/user"
	"net/http"
	"net/url"
	"time"
)

type imagePolicyPorts struct {
	targets        func(context.Context, string) ([]updateTarget, error)
	checkContainer func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error)
	admitContainer func(context.Context, string, string, string, dto.PolicyBinding) (activity.RuntimeAdmissionResult, error)
	hints          func(context.Context, string) (dto.SystemImageHints, error)
	host           func(context.Context, string) (string, error)
	actor          func(context.Context, string, string, string, bool) (context.Context, error)
	baseline       func(context.Context, string, string) (dto.BaselineStatus, error)
	check          func(context.Context, string, string) (*projectDTO.UpdateInfo, error)
	admit          func(context.Context, string, string, string, dto.LifecycleRequest, dto.PolicyBinding) (activity.RuntimeAdmissionResult, error)
	wait           func(context.Context, string, string) (activityDTO.RuntimeSnapshot, error)
	snapshot       func(context.Context, string, string) (activityDTO.RuntimeSnapshot, bool, error)
	notify         func(context.Context, string, string, dto.UpdateCandidate) (int, error)
}

func UpdateWindowJobKey(env, kind, target string) string {
	return "update-window:" + env + ":" + kind + ":" + target
}

func (s *UpdaterService) ScheduledUpdatePolicies(ctx context.Context) ([]policy.Policy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return nil, err
	}
	var rows []policy.Policy
	err = p.store.DB.WithContext(ctx).Where("mode = ?", "scheduled").Order("environment_id,target_kind,target_id").Find(&rows).Error
	return rows, err
}

func (s *UpdaterService) UpdateCheckSchedule(ctx context.Context) string {
	switch s.deps.Settings.GetStringSetting(ctx, "updateCheckInterval", "12h") {
	case "6h":
		return "0 0 */6 * * *"
	case "24h":
		return "0 0 0 * * *"
	default:
		return "0 0 */12 * * *"
	}
}

type imagePolicyService struct {
	startedAt  time.Time
	store      policy.Store
	ports      imagePolicyPorts
	reschedule func(context.Context) error
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
func (s *UpdaterService) ConfigureImagePolicies(envs *environment.EnvironmentService, reschedule func(context.Context) error, containers ...*containerService.ContainerService) {
	if s.config != nil && (s.config.AgentMode || s.config.EdgeAgent) {
		return
	}
	p := &imagePolicyService{store: policy.Store{DB: s.deps.DB.DB}, reschedule: reschedule, startedAt: time.Now().UTC()}
	var inventory *containerService.ContainerService
	if len(containers) > 0 {
		inventory = containers[0]
	}
	p.ports.targets = s.policyTargets(envs, inventory)
	p.ports.checkContainer = s.policyContainerCheck(envs)
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
	p.ports.baseline = s.imagePolicyBaseline(envs)
	p.ports.check = func(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
		if env == "0" {
			return s.CheckProjectUpdates(ctx, target)
		}
		var out base.ApiResponse[*projectDTO.UpdateInfo]
		err := envs.ProxyJSONRequest(ctx, env, http.MethodPost, "/api/environments/0/updater/projects/"+url.PathEscape(target)+"/check", nil, &out)
		if refusal := relayedCheckRefusal(err); refusal != nil {
			return nil, refusal
		}
		if err != nil {
			return nil, err
		}
		return out.Data, nil
	}
	p.ports.admit = s.deps.Activity.AdmitPolicyUpdate
	p.ports.admitContainer = s.deps.Activity.AdmitPolicyContainerUpdate
	p.ports.wait = s.deps.Activity.WaitPolicyUpdate
	p.ports.snapshot = s.deps.Activity.PolicyUpdateSnapshot
	if s.deps.Notifications != nil {
		p.ports.notify = s.deps.Notifications.NotifyImagePolicyCandidate
	}
	s.policies = p
	if s.deps.ImageUpdates != nil {
		s.deps.ImageUpdates.ConfigureHubReads(envs.ProxyJSONRequest, s.observeManualImages)
	}
}
func (s *UpdaterService) imagePolicyActor(ctx context.Context, env, actor, key string, apply bool) (context.Context, error) {
	if s.roles == nil || actor == "" {
		return ctx, huma.Error403Forbidden("Policy actor unavailable")
	}
	ps, err := s.roles.ResolveExecutionPermissions(ctx, actor, key)
	if err != nil {
		return ctx, huma.Error403Forbidden("Policy actor unavailable")
	}
	if apply && !ps.IsGlobalAdmin() {
		return ctx, imageDTO.Failure("update.permission-denied", 403)
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
func (s *UpdaterService) GetImagePolicy(ctx context.Context, env, target string, kinds ...string) (dto.AutoUpdatePolicy, error) {
	return s.imagePolicy(ctx, env, target, true, kinds...)
}

// imagePolicy reads one policy row. With live set, a default (revision 0) stack policy also resolves the target's
// current identity through the baseline port (Compose inputs and Docker observation); lists skip that projection.
func (s *UpdaterService) imagePolicy(ctx context.Context, env, target string, live bool, kinds ...string) (dto.AutoUpdatePolicy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	row, err := p.store.Policy(ctx, env, reg, target, kinds...)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	if live && row.Revision == 0 && row.TargetKind == "stack" {
		if live, e := p.ports.baseline(ctx, env, target); e == nil && live.Valid() {
			row.TargetIdentity = live.TargetIdentity
		}
	}
	out := dto.AutoUpdatePolicy{Mode: row.Mode, Window: row.Window(), Revision: row.Revision, EnvironmentID: env, RegistrationID: reg, TargetKind: row.TargetKind, TargetID: target, TargetName: target, TargetIdentity: row.TargetIdentity, JobID: "update-check", CheckedAt: row.CheckedAt, Code: row.Code, ApplyCheckedAt: row.ApplyCheckedAt, ApplyCode: row.ApplyCode, BookkeepingAt: row.BookkeepingAt, BookkeepingCode: row.BookkeepingCode, Candidates: []dto.UpdateCandidate{}}
	if row.Mode == "scheduled" {
		out.JobID = UpdateWindowJobKey(row.EnvironmentID, row.TargetKind, row.TargetID)
	}
	out.Stale = row.CheckedAt == nil || row.Code != ""
	out.TargetMissing = row.MissingSince != nil
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
func (s *UpdaterService) SaveImagePolicy(ctx context.Context, env, target string, in dto.UpdatePolicyInput, actor, key string, kinds ...string) (dto.AutoUpdatePolicy, error) {
	p, err := s.hubPolicies()
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	if in.Mode != "check" && in.Mode != "scheduled" {
		return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.invalid-policy", 400)
	}
	if _, err = p.ports.actor(ctx, env, actor, key, in.Mode == "scheduled"); err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	reg, err := p.ports.host(ctx, env)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	row, err := p.store.Policy(ctx, env, reg, target, kinds...)
	if err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	// Disabling a known policy needs no executor observation, including offline or
	// since origin-locked targets. Enabling/rebinding still proves the target afresh.
	if row.TargetKind == "container" {
		live, e := p.findContainerTarget(ctx, env, target)
		if e != nil && (in.Mode != "check" || row.Revision == 0) {
			return dto.AutoUpdatePolicy{}, e
		}
		if e == nil {
			row.TargetIdentity = live.Name
		}
	} else if in.Mode == "scheduled" {
		baseline, e := p.ports.baseline(ctx, env, target)
		if e != nil {
			return dto.AutoUpdatePolicy{}, e
		}
		if !baseline.Valid() {
			return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.check-unavailable", 503)
		}
		row.TargetIdentity = baseline.TargetIdentity
	}
	if in.Mode == "scheduled" && !in.Window.Valid() || in.Mode == "check" && in.Window != nil {
		return dto.AutoUpdatePolicy{}, imageDTO.Failure("update.invalid-policy", 400)
	}
	row.SetWindow(in.Window)
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
	if err = s.RefreshUpdateWindows(ctx); err != nil {
		return dto.AutoUpdatePolicy{}, err
	}
	return s.GetImagePolicy(ctx, env, target, kinds...)
}
func (s *UpdaterService) RefreshUpdateWindows(ctx context.Context) error {
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
	var host *environmentDTO.HostProblem
	if errors.As(err, &host) && imageDTO.ValidReason(host.Code) {
		return host.Code
	}
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
	if err := s.deps.DB.WithContext(ctx).Table("environments").Where("enabled = ?", true).Order("id").Pluck("id", &ids).Error; err != nil {
		return nil, err
	}
	out := []string{}
	for _, id := range ids {
		out = append(out, id)
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
