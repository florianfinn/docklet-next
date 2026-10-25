// Source: docklet-next/docs/design/update-policies.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Migrated hub store and synchronous effect/receipt ports prove policy boundaries.

package updater

import (
	"context"
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	a "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/google/uuid"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

type policyFixture struct {
	s                                                                           *UpdaterService
	p                                                                           *imagePolicyService
	db                                                                          *database.DB
	url                                                                         string
	baseline                                                                    dto.BaselineStatus
	check                                                                       *projectDTO.UpdateInfo
	checks, admissions, waits, notifications, snapshots, actors, scheduleActors int
	revoked, scheduleRevoked                                                    bool
	beforeAdmission                                                             func(dto.PolicyBinding)
	afterAdmission                                                              func(dto.PolicyBinding)
	wait                                                                        func(string) (a.RuntimeSnapshot, error)
	receipts                                                                    map[string]a.RuntimeSnapshot
	bindings                                                                    []dto.PolicyBinding
	targets                                                                     []string
}

func newPolicyFixture(t *testing.T) *policyFixture {
	t.Helper()
	f := &policyFixture{url: sqlitestore.URL(filepath.Join(t.TempDir(), "policies.db")), receipts: map[string]a.RuntimeSnapshot{}}
	var err error
	f.db, err = database.InitializeHub(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, f.db.Close()) })
	cfg, err := settings.NewSettingsService(t.Context(), f.db)
	require.NoError(t, err)
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, cfg.Stop(ctx))
	})
	require.NoError(t, cfg.UpdateSetting(t.Context(), "autoUpdate", "true"))
	require.NoError(t, cfg.UpdateSetting(t.Context(), "pollingEnabled", "true"))
	f.s = &UpdaterService{config: &config.Config{}, deps: updaterDependenciesInternal{DB: f.db, Settings: cfg}}
	f.p = &imagePolicyService{store: policy.Store{DB: f.db.DB}}
	f.s.policies = f.p
	f.baseline = dto.BaselineStatus{Available: true, TargetIdentity: strings.Repeat("a", 64), SavedRevision: strings.Repeat("b", 64)}
	f.check = &projectDTO.UpdateInfo{Status: "has_update", HasUpdate: true, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/app:1.0.0", UpdateInfo: &imageDTO.UpdateInfo{HasUpdate: true, State: "version", LatestVersion: "v1.1.0", LatestDigest: "sha256:" + strings.Repeat("c", 64), Platform: "linux/amd64"}}}}
	f.p.ports.host = func(context.Context, string) (string, error) { return "example-registration", nil }
	f.p.ports.actor = func(ctx context.Context, env, actor, key string, apply bool) (context.Context, error) {
		f.actors++
		if f.revoked {
			return ctx, huma.Error403Forbidden("Synthetic revoked grants")
		}
		return ctx, nil
	}
	f.p.ports.scheduleActor = func(context.Context, string, string, string) error {
		f.scheduleActors++
		if f.scheduleRevoked {
			return huma.Error403Forbidden("Synthetic revoked schedule grants")
		}
		return nil
	}
	f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) { return f.baseline, nil }
	f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
		f.checks++
		return f.check, nil
	}
	f.p.ports.admit = func(ctx context.Context, env, target, key string, request dto.LifecycleRequest, b dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
		if f.beforeAdmission != nil {
			f.beforeAdmission(b)
		}
		err := f.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
			if e := policy.ValidateAdmission(tx, &b, env, "example-registration", target, f.baseline.TargetIdentity, "example-actor"); e != nil {
				return e
			}
			return policy.AdmitChild(tx, &b, b.ChildID)
		})
		if err != nil {
			return activity.RuntimeAdmissionResult{}, err
		}
		f.admissions++
		f.bindings = append(f.bindings, b)
		f.targets = append(f.targets, target)
		f.receipts[b.ChildID] = a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusSuccess}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
		if f.afterAdmission != nil {
			f.afterAdmission(b)
		}
		return activity.RuntimeAdmissionResult{ActivityID: b.ChildID}, nil
	}
	f.p.ports.wait = func(_ context.Context, env, id string) (a.RuntimeSnapshot, error) {
		f.waits++
		if f.wait != nil {
			return f.wait(id)
		}
		return f.receipts[id], nil
	}
	f.p.ports.snapshot = func(_ context.Context, env, id string) (a.RuntimeSnapshot, bool, error) {
		f.snapshots++
		v, ok := f.receipts[id]
		return v, ok, nil
	}
	f.p.ports.notify = func(_ context.Context, env, id string, c dto.UpdateCandidate) (int, error) {
		f.notifications++
		return 1, nil
	}
	schedule, err := f.p.store.Schedule(t.Context(), "0", "example-registration")
	require.NoError(t, err)
	schedule.ActorID = "example-schedule-actor"
	require.NoError(t, f.p.store.SaveSchedule(t.Context(), schedule, 0))
	return f
}
func (f *policyFixture) save(t *testing.T, target, mode string) policy.Policy {
	t.Helper()
	p, err := f.p.store.Policy(t.Context(), "0", "example-registration", target)
	require.NoError(t, err)
	p.Mode = mode
	p.TargetIdentity = f.baseline.TargetIdentity
	p.ActorID = "example-actor"
	p.ActorKeyID = "example-key"
	require.NoError(t, f.p.store.SavePolicy(t.Context(), p, p.Revision))
	p.Revision++
	return p
}
func (f *policyFixture) run(t *testing.T, kind string, run st.Run) st.Outcome {
	t.Helper()
	if run.ID == "" {
		run.ID = uuid.NewString()
	}
	if run.AttemptCount == 0 {
		run.AttemptCount = 1
	}
	out, err := f.s.RunImagePolicyHost(jobcontext.WithExecution(t.Context(), run, nil), "0", kind)
	require.NoError(t, err)
	return out
}
func (f *policyFixture) reopen(t *testing.T) {
	t.Helper()
	require.NoError(t, f.s.deps.Settings.Stop(t.Context()))
	require.NoError(t, f.db.Close())
	var err error
	f.db, err = database.InitializeHub(t.Context(), f.url, database.MigrationOptions{})
	require.NoError(t, err)
	f.p.store.DB = f.db.DB
	f.s.deps.DB = f.db
	cfg, err := settings.NewSettingsService(t.Context(), f.db)
	require.NoError(t, err)
	f.s.deps.Settings = cfg
	t.Cleanup(func() {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, cfg.Stop(ctx))
	})
}
func TestImagePolicyModes(t *testing.T) {
	for _, mode := range []string{"off", "notify", "apply"} {
		t.Run(mode, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "example-stack", mode)
			out := f.run(t, "auto-update", st.Run{})
			require.Len(t, out.Targets, 1)
			if mode != "apply" {
				require.Zero(t, f.checks)
				require.Zero(t, f.admissions)
			} else {
				require.Equal(t, 1, f.admissions)
				require.Equal(t, 1, f.waits)
				require.Equal(t, st.Succeeded, out.Targets[0].Status)
			}
			require.Zero(t, f.notifications)
			if mode == "off" {
				f.run(t, "image-polling", st.Run{})
				require.Zero(t, f.checks)
				require.Zero(t, f.notifications)
			}
			if mode == "notify" {
				out = f.run(t, "image-polling", st.Run{})
				require.Equal(t, st.Succeeded, out.Targets[0].Status)
				require.Equal(t, 1, f.notifications)
				require.Zero(t, f.admissions)
			}
		})
	}
}
func TestImagePolicyAdmissionRefusals(t *testing.T) {
	tests := []struct {
		name, code string
		setup      func(*testing.T, *policyFixture)
	}{
		{"missing baseline", "update.baseline-unavailable", func(_ *testing.T, f *policyFixture) { f.baseline.Available = false }},
		{"saved drift", "update.config-drift", func(_ *testing.T, f *policyFixture) {
			f.baseline.Available = false
			f.baseline.Code = "update.config-drift"
		}},
		{"excluded service", "update.auto-excluded", func(_ *testing.T, f *policyFixture) { f.baseline.AutoExcluded = true }},
		{"replacement target", "update.target-changed", func(_ *testing.T, f *policyFixture) { f.baseline.TargetIdentity = strings.Repeat("d", 64) }},
		{"revoked policy actor", "FORBIDDEN", func(_ *testing.T, f *policyFixture) { f.revoked = true }},
		{"unknown observation", "update.check-unavailable", func(_ *testing.T, f *policyFixture) {
			f.check.Status = "unknown"
			f.check.ServiceUpdates = nil
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "example-stack", "apply")
			tt.setup(t, f)
			out := f.run(t, "auto-update", st.Run{})
			require.Len(t, out.Targets, 1)
			require.Equal(t, tt.code, out.Targets[0].Code)
			require.Zero(t, f.admissions)
			var children int64
			require.NoError(t, f.db.Model(&policy.Child{}).Count(&children).Error)
			require.Zero(t, children)
		})
	}
}
func TestImagePolicyMasterAndScheduleActorGates(t *testing.T) {
	for _, gate := range []string{"autoUpdate", "pollingEnabled", "actor"} {
		t.Run(gate, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "example-stack", "apply")
			if gate == "actor" {
				f.scheduleRevoked = true
			} else {
				require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), gate, "false"))
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Zero(t, f.checks)
			require.Zero(t, f.admissions)
			if gate == "actor" {
				require.Equal(t, "FORBIDDEN", out.Code)
			} else {
				require.Equal(t, st.Skipped, out.Status)
			}
		})
	}
}
func TestImagePolicyChangedBeforeAdmissionRefuses(t *testing.T) {
	f := newPolicyFixture(t)
	p := f.save(t, "example-stack", "apply")
	f.beforeAdmission = func(dto.PolicyBinding) { f.save(t, "example-stack", "off") }
	out := f.run(t, "auto-update", st.Run{})
	require.Zero(t, f.admissions)
	require.Zero(t, f.waits)
	require.Equal(t, "update.policy-changed", out.Targets[0].Code)
	var c policy.Child
	require.NoError(t, f.db.First(&c, "policy_id = ?", p.ID).Error)
	require.Equal(t, "refused", c.Status)
}
func TestImagePolicyFrozenAfterAdmissionAndSerialChildren(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	f.afterAdmission = func(b dto.PolicyBinding) {
		if f.admissions == 1 {
			f.save(t, "a-stack", "off")
		}
	}
	f.wait = func(id string) (a.RuntimeSnapshot, error) {
		require.Equal(t, f.waits, f.admissions, "the next child cannot be admitted before the prior waiter observes settlement")
		return f.receipts[id], nil
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, []string{"a-stack", "b-stack"}, f.targets)
	require.Equal(t, 2, f.waits)
	require.Equal(t, st.Succeeded, out.Status)
	require.EqualValues(t, 1, f.bindings[0].Revision, "accepted revision is frozen despite subsequent off")
}
func TestImagePolicyRevalidatesBeforeEachChild(t *testing.T) {
	for _, actor := range []string{"policy", "schedule"} {
		t.Run(actor, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "a-stack", "apply")
			f.save(t, "b-stack", "apply")
			f.wait = func(id string) (a.RuntimeSnapshot, error) {
				if actor == "policy" {
					f.revoked = true
				} else {
					f.scheduleRevoked = true
				}
				return f.receipts[id], nil
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Equal(t, 1, f.admissions)
			require.Len(t, out.Targets, 2)
			require.Equal(t, "FORBIDDEN", out.Targets[1].Code)
		})
	}
}
func TestImagePolicyUnknownChildStopsHost(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	f.wait = func(string) (a.RuntimeSnapshot, error) {
		return a.RuntimeSnapshot{RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeUnknown}}, nil
	}
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, 1, f.admissions)
	require.Len(t, out.Targets, 1)
	require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
}
func TestImagePolicyRestartRetryAndCheckpointNeverReplay(t *testing.T) {
	for _, boundary := range []string{"prepared", "admitted", "missed", "retry", "duplicate"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			p := f.save(t, "example-stack", "apply")
			run := st.Run{ID: uuid.NewString(), AttemptCount: 1}
			if boundary == "prepared" || boundary == "admitted" {
				child, _, err := f.p.store.PrepareChild(t.Context(), p, run.ID)
				require.NoError(t, err)
				if boundary == "admitted" {
					require.NoError(t, policy.AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
				}
			}
			if boundary == "duplicate" {
				f.run(t, "auto-update", run)
				require.Equal(t, 1, f.admissions)
			}
			before := f.admissions
			if boundary != "duplicate" {
				run.Trigger = "recovery"
			}
			if boundary == "retry" {
				run.AttemptCount = 2
				run.Trigger = "manual"
			}
			f.reopen(t)
			out := f.run(t, "auto-update", run)
			require.Equal(t, before, f.admissions, "restart/retry/checkpoint cannot create another runtime effect")
			if boundary == "admitted" {
				require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
			}
			if boundary == "missed" {
				require.Zero(t, f.checks)
				require.Equal(t, st.Skipped, out.Status)
			}
		})
	}
}
func TestImagePolicyNotificationDeliveryAndRestartDedup(t *testing.T) {
	for _, delivery := range []string{"sent", "unconfigured", "failed"} {
		t.Run(delivery, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "example-stack", "notify")
			f.p.ports.notify = func(_ context.Context, env, id string, c dto.UpdateCandidate) (int, error) {
				f.notifications++
				if delivery == "failed" {
					return 0, errors.New("synthetic private provider detail")
				}
				if delivery == "unconfigured" {
					return 0, nil
				}
				return 1, nil
			}
			f.run(t, "image-polling", st.Run{})
			first, err := f.s.GetImagePolicy(t.Context(), "0", "example-stack")
			require.NoError(t, err)
			require.Len(t, first.Candidates, 1)
			require.Equal(t, delivery, first.Candidates[0].NotificationState)
			require.False(t, first.Stale)
			require.Equal(t, "1.1.0", first.Candidates[0].Version)
			f.reopen(t)
			f.save(t, "example-stack", "off")
			f.save(t, "example-stack", "notify")
			for range 2 {
				_, err = f.s.CheckPolicyProject(t.Context(), "0", "example-stack")
				require.NoError(t, err)
				f.run(t, "image-polling", st.Run{})
			}
			require.Equal(t, 1, f.notifications)
			var count int64
			require.NoError(t, f.db.Model(&policy.Notice{}).Count(&count).Error)
			require.EqualValues(t, 1, count)
			repeated, err := f.s.GetImagePolicy(t.Context(), "0", "example-stack")
			require.NoError(t, err)
			require.Equal(t, first.Candidates[0].EntryID, repeated.Candidates[0].EntryID)
			f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest = "sha256:" + strings.Repeat("e", 64)
			f.run(t, "image-polling", st.Run{})
			require.Equal(t, 2, f.notifications)
		})
	}
}
func TestImagePolicyAPIConfigurationCannotApply(t *testing.T) {
	f := newPolicyFixture(t)
	out, err := f.s.GetImagePolicy(t.Context(), "0", "example-stack")
	require.NoError(t, err)
	require.Equal(t, "off", out.Mode)
	require.True(t, out.Stale)
	_, err = f.s.SaveImagePolicy(t.Context(), "0", "example-stack", dto.UpdatePolicyInput{Mode: "apply"}, "example-actor", "example-key")
	require.NoError(t, err)
	require.Zero(t, f.checks)
	require.Zero(t, f.admissions)
	require.Zero(t, f.notifications)
}

func TestImagePolicyRevokedDuringCheckCannotNotify(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "example-stack", "notify")
	check := f.p.ports.check
	f.p.ports.check = func(ctx context.Context, env, target string) (*projectDTO.UpdateInfo, error) {
		result, err := check(ctx, env, target)
		f.revoked = true
		return result, err
	}
	out := f.run(t, "image-polling", st.Run{})
	require.Equal(t, "FORBIDDEN", out.Targets[0].Code)
	require.Zero(t, f.notifications)
	var count int64
	require.NoError(t, f.db.Model(&policy.Notice{}).Count(&count).Error)
	require.Zero(t, count)
}
func TestImagePolicyCanBeDisabledWithoutOfflineObservation(t *testing.T) {
	f := newPolicyFixture(t)
	p := f.save(t, "example-stack", "apply")
	f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) {
		return dto.BaselineStatus{}, errors.New("synthetic offline target")
	}
	result, err := f.s.SaveImagePolicy(t.Context(), "0", "example-stack", dto.UpdatePolicyInput{Mode: "off", ExpectedRevision: p.Revision}, "example-actor", "example-key")
	require.NoError(t, err)
	require.Equal(t, "off", result.Mode)
	require.Equal(t, p.Revision+1, result.Revision)
	require.Zero(t, f.admissions)
}
func TestImagePolicyBusyAndInvalidCandidateNeverApply(t *testing.T) {
	for _, state := range []string{"busy", "missing platform", "invalid digest"} {
		t.Run(state, func(t *testing.T) {
			f := newPolicyFixture(t)
			f.save(t, "example-stack", "apply")
			if state == "busy" {
				f.p.ports.admit = func(context.Context, string, string, string, dto.LifecycleRequest, dto.PolicyBinding) (activity.RuntimeAdmissionResult, error) {
					return activity.RuntimeAdmissionResult{}, imageDTO.Failure("image.busy", 409)
				}
			} else if state == "missing platform" {
				f.check.ServiceUpdates["app"].UpdateInfo.Platform = ""
			} else {
				f.check.ServiceUpdates["app"].UpdateInfo.LatestDigest = "sha256:" + strings.Repeat("z", 64)
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Len(t, out.Targets, 1)
			if state == "busy" {
				require.Equal(t, st.Skipped, out.Targets[0].Status)
				require.Equal(t, "image.busy", out.Targets[0].Code)
			} else {
				require.Equal(t, "update.check-unavailable", out.Targets[0].Code)
			}
			require.Zero(t, f.admissions)
		})
	}
}

func TestImagePolicyQueuedBeforeHubStartDoesNotCatchUp(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "example-stack", "apply")
	f.p.startedAt = time.Now().UTC()
	out := f.run(t, "auto-update", st.Run{ID: uuid.NewString(), AttemptCount: 1, Trigger: "scheduled", CreatedAt: f.p.startedAt.Add(-time.Minute)})
	require.Equal(t, st.Skipped, out.Status)
	require.Zero(t, f.checks)
	require.Zero(t, f.admissions)
}

func TestImagePolicyUnknownChildBlocksLaterHostBatchesUntilReceipt(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-stack", "apply")
	f.save(t, "b-stack", "apply")
	f.wait = func(id string) (a.RuntimeSnapshot, error) {
		snapshot := a.RuntimeSnapshot{RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeUnknown}}
		f.receipts[id] = snapshot
		return snapshot, nil
	}
	first := f.run(t, "auto-update", st.Run{})
	require.Equal(t, 1, f.admissions)
	firstID := first.Targets[0].ActivityID
	before := f.checks
	f.reopen(t)
	blocked := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "runtime.outcome-unknown", blocked.Code)
	require.Equal(t, firstID, blocked.Targets[0].ActivityID)
	require.Equal(t, 1, f.admissions)
	require.Equal(t, before, f.checks, "another wrapper cannot bypass the prior child's host boundary")
	f.receipts[firstID] = a.RuntimeSnapshot{Activity: a.Activity{Status: a.StatusSuccess}, RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeKnown}}
	f.wait = nil
	resumed := f.run(t, "auto-update", st.Run{})
	require.Equal(t, st.Succeeded, resumed.Status)
	require.Equal(t, 3, f.admissions)
	var child policy.Child
	require.NoError(t, f.db.First(&child, "id = ?", firstID).Error)
	require.Equal(t, "settled", child.Status)
}

func TestImagePolicyUnknownHostDoesNotBlockAnotherHost(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "local-stack", "apply")
	f.wait = func(id string) (a.RuntimeSnapshot, error) {
		if f.admissions == 1 {
			unknown := a.RuntimeSnapshot{RuntimeKnowledge: a.RuntimeKnowledge{OutcomeState: a.RuntimeOutcomeUnknown}}
			f.receipts[id] = unknown
			return unknown, nil
		}
		return f.receipts[id], nil
	}
	local := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "runtime.outcome-unknown", local.Targets[0].Code)
	env := "example-remote"
	require.NoError(t, f.db.Create(&environment.Environment{BaseModel: database.BaseModel{ID: env}, Name: "Example remote", ApiUrl: "https://agent.example.com", Enabled: true, Status: "online"}).Error)
	schedule, err := f.p.store.Schedule(t.Context(), env, "example-registration")
	require.NoError(t, err)
	schedule.ActorID = "example-schedule-actor"
	require.NoError(t, f.p.store.SaveSchedule(t.Context(), schedule, 0))
	one, err := f.p.store.Policy(t.Context(), env, "example-registration", "remote-stack")
	require.NoError(t, err)
	one.Mode = "apply"
	one.ActorID = "example-actor"
	one.TargetIdentity = f.baseline.TargetIdentity
	require.NoError(t, f.p.store.SavePolicy(t.Context(), one, 0))
	run := st.Run{ID: uuid.NewString(), AttemptCount: 1, CreatedAt: time.Now().UTC()}
	remote, err := f.s.RunImagePolicyHost(jobcontext.WithExecution(t.Context(), run, nil), env, "auto-update")
	require.NoError(t, err)
	require.Equal(t, st.Succeeded, remote.Status)
	require.Equal(t, 2, f.admissions)
}
