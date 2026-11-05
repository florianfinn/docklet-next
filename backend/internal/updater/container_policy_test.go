// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original container name identity, host isolation and dedup proofs.

package updater

import (
	"context"
	"errors"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestContainerPolicyFollowsNameAndProjectsMissingTarget(t *testing.T) {
	f := newPolicyFixture(t)
	name, id := "app", "first-instance"
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: name, Name: name, ContainerID: id, Eligible: true}}, nil
	}
	saved, e := f.s.SaveImagePolicy(t.Context(), "0", name, dto.UpdatePolicyInput{Mode: "scheduled", Window: &dto.UpdateWindow{Kind: "daily", Time: "04:00"}}, "example-actor", "", "container")
	require.NoError(t, e)
	id = "replacement-instance"
	got, e := f.s.GetImagePolicy(t.Context(), "0", name, "container")
	require.NoError(t, e)
	require.Equal(t, saved.Revision, got.Revision)
	require.Equal(t, "scheduled", got.Mode)
	require.Equal(t, name, got.TargetID)
	name = "renamed"
	newPolicy, e := f.s.GetImagePolicy(t.Context(), "0", name, "container")
	require.NoError(t, e)
	require.Equal(t, "check", newPolicy.Mode)
	require.Zero(t, newPolicy.Revision)
	list, e := f.s.ListImagePolicies(t.Context(), "0")
	require.NoError(t, e)
	require.Len(t, list, 2)
	require.Equal(t, "app", list[1].TargetID)
	require.True(t, list[1].TargetMissing)
}

// A scheduled policy whose container later became origin-locked can still be
// switched back to check; it can never be re-enabled.
func TestLockedContainerPolicyCanOnlyBeDisabled(t *testing.T) {
	f := newPolicyFixture(t)
	eligible := true
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "example-id", Eligible: eligible}}, nil
	}
	window := &dto.UpdateWindow{Kind: "daily", Time: "04:00"}
	saved, e := f.s.SaveImagePolicy(t.Context(), "0", "app", dto.UpdatePolicyInput{Mode: "scheduled", Window: window}, "example-actor", "", "container")
	require.NoError(t, e)
	eligible = false
	_, e = f.s.SaveImagePolicy(t.Context(), "0", "app", dto.UpdatePolicyInput{Mode: "scheduled", Window: window, ExpectedRevision: saved.Revision}, "example-actor", "", "container")
	require.Equal(t, "container.origin-locked", policyCode(e, ""))
	disabled, e := f.s.SaveImagePolicy(t.Context(), "0", "app", dto.UpdatePolicyInput{Mode: "check", ExpectedRevision: saved.Revision}, "example-actor", "", "container")
	require.NoError(t, e)
	require.Equal(t, "check", disabled.Mode)
}

func TestContainerPolicyRevisionIsFrozenAtAdmission(t *testing.T) {
	f := newPolicyFixture(t)
	row, e := f.p.store.Policy(t.Context(), "0", "example-registration", "app", "container")
	require.NoError(t, e)
	row.Mode = "scheduled"
	row.ActorID = "example-actor"
	row.TargetIdentity = "app"
	row.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, f.p.store.SavePolicy(t.Context(), row, 0))
	row.Revision = 1
	child, _, e := f.p.store.PrepareChild(t.Context(), row, "window")
	require.NoError(t, e)
	binding := &dto.PolicyBinding{PolicyID: row.ID, Revision: 1, JobRunID: "window", ChildID: child.ID}
	require.NoError(t, f.db.Transaction(func(tx *gorm.DB) error {
		return policy.ValidateAdmission(tx, binding, "0", "example-registration", "app", "app", "example-actor", "container")
	}))
	row.Mode = "scheduled"
	row.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "05:00"})
	require.NoError(t, f.p.store.SavePolicy(t.Context(), row, 1))
	e = f.db.Transaction(func(tx *gorm.DB) error {
		return policy.ValidateAdmission(tx, binding, "0", "example-registration", "app", "app", "example-actor", "container")
	})
	require.Equal(t, "update.policy-changed", policyCode(e, ""))
}

func TestHostFailureDoesNotPruneOrStopOtherHosts(t *testing.T) {
	f := newPolicyFixture(t)
	row := f.save(t, "missing", "check")
	old := time.Now().UTC().Add(-8 * 24 * time.Hour)
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id=?", row.ID).Update("missing_since", old).Error)
	require.NoError(t, f.db.Exec("INSERT INTO environments(id,name,api_url,enabled) VALUES('z-live','Live','https://agent.example.com',1)").Error)
	f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
		if env == "0" {
			return nil, errors.New("partial or offline inventory")
		}
		return []updateTarget{{Kind: "stack", ID: "live", Name: "Live", Eligible: true}}, nil
	}
	out, e := f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	require.Equal(t, st.Partial, out.Status)
	require.Len(t, out.Targets, 2)
	require.Equal(t, "agent.unavailable", out.Targets[0].Code)
	require.Equal(t, st.Succeeded, out.Targets[1].Status)
	require.NoError(t, f.db.First(&row, "id=?", row.ID).Error)
	require.WithinDuration(t, old, *row.MissingSince, time.Millisecond)
}

func TestContainerCandidateIdentityAndNotifyToggle(t *testing.T) {
	f := newPolicyFixture(t)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app-a", Name: "app-a", Eligible: true}, {Kind: "container", ID: "app-b", Name: "app-b", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return f.check, nil }
	require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "false"))
	_, e := f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	var n int64
	require.NoError(t, f.db.Model(&policy.Notice{}).Count(&n).Error)
	require.Zero(t, n)
	require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "true"))
	_, e = f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	require.Equal(t, 2, f.notifications)
	f.reopen(t)
	_, e = f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	require.Equal(t, 2, f.notifications)
	require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "false"))
	_, e = f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	require.NoError(t, f.s.deps.Settings.UpdateSetting(t.Context(), "updateNotify", "true"))
	_, e = f.s.RunUpdateCheck(t.Context())
	require.NoError(t, e)
	require.Equal(t, 2, f.notifications)
}

func TestContainerWindowRestartReconcilesWithoutAdmission(t *testing.T) {
	f := newPolicyFixture(t)
	row, e := f.p.store.Policy(t.Context(), "0", "example-registration", "app", "container")
	require.NoError(t, e)
	row.Mode = "scheduled"
	row.SetWindow(&dto.UpdateWindow{Kind: "daily", Time: "04:00"})
	require.NoError(t, f.p.store.SavePolicy(t.Context(), row, 0))
	row.Revision = 1
	child, _, e := f.p.store.PrepareChild(t.Context(), row, "window")
	require.NoError(t, e)
	require.NoError(t, policy.AdmitChild(f.db.DB, &dto.PolicyBinding{ChildID: child.ID}, child.ID))
	f.reopen(t)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "app", Name: "app", ContainerID: "instance", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) {
		f.checks++
		return f.check, nil
	}
	out, e := f.s.RunUpdateWindow(jobcontext.WithExecution(t.Context(), st.Run{ID: "window", AttemptCount: 2}, nil), row.ID)
	require.NoError(t, e)
	require.Len(t, out.Targets, 1)
	require.Equal(t, "runtime.outcome-unknown", out.Targets[0].Code)
	require.Zero(t, f.admissions)
	require.Zero(t, f.checks)
}
