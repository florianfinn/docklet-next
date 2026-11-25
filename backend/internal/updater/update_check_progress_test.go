// Source: docklet-next/issues/626
// Revision: 6dc802124ce20a9c53693493ca66a85d1820ef19
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original M41 coverage for update-check plans, names, missing local images and the check aggregate.

package updater

import (
	"context"
	"errors"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
)

type planCall struct {
	ids   []string
	known bool
}

// progressContext records what the job persists through the execution context, as the coordinator would.
func progressContext(t *testing.T, plans *[]planCall, outcome *st.Outcome) context.Context {
	ctx := jobcontext.WithExecution(t.Context(), st.Run{ID: "example-run", AttemptCount: 1}, func(target st.TargetOutcome) error {
		for i := range outcome.Targets {
			if outcome.Targets[i].ID == target.ID {
				outcome.Targets[i] = target
				return nil
			}
		}
		outcome.Targets = append(outcome.Targets, target)
		return nil
	})
	return jobcontext.WithPlan(ctx, func(ids []string, known bool) error {
		*plans = append(*plans, planCall{ids: append([]string(nil), ids...), known: known})
		outcome.PlannedIDs, outcome.PlannedKnown = ids, known
		return nil
	})
}

func perTargetChecks(f *policyFixture, checks map[string]*projectDTO.UpdateInfo) {
	f.p.ports.check = func(_ context.Context, _ string, target string) (*projectDTO.UpdateInfo, error) {
		f.checks++
		return checks[target], nil
	}
}

func upToDate() *projectDTO.UpdateInfo {
	return &projectDTO.UpdateInfo{Status: "up_to_date", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/app:1.0.0", UpdateInfo: &imageDTO.UpdateInfo{State: "version", Platform: "linux/amd64"}}}}
}

func TestHostCheckSkipsMissingLocalImageAndCountsPlannedTargets(t *testing.T) {
	f := newPolicyFixture(t)
	for _, target := range []string{"a-missing", "b-update", "c-current"} {
		f.save(t, target, "check")
	}
	notPulled := &projectDTO.UpdateInfo{Status: "not_pulled", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/missing:1.0.0"}}}
	perTargetChecks(f, map[string]*projectDTO.UpdateInfo{"a-missing": notPulled, "b-update": f.check, "c-current": upToDate()})
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)

	require.Equal(t, st.Succeeded, out.Status)
	byTarget := map[string]st.TargetOutcome{}
	for _, target := range out.Targets {
		byTarget[target.TargetID] = target
	}
	require.Equal(t, st.Skipped, byTarget["a-missing"].Status)
	require.Equal(t, "update.image-not-pulled", byTarget["a-missing"].Code)
	require.Equal(t, "a-missing", byTarget["a-missing"].Name)
	require.True(t, byTarget["b-update"].UpdateAvailable)
	require.False(t, byTarget["c-current"].UpdateAvailable)
	require.Zero(t, f.admissions)

	require.Len(t, plans, 1)
	require.True(t, plans[0].known)
	require.ElementsMatch(t, []string{"0~stack~a-missing", "0~stack~b-update", "0~stack~c-current"}, plans[0].ids)
	counts, ok := persisted.Counts()
	require.True(t, ok)
	require.Equal(t, st.BulkCounts{Planned: 3, PlannedKnown: true, Completed: 3, Succeeded: 2, Skipped: 1, Updates: 1}, counts)
}

func TestHostCheckGenericUnavailableStaysFailed(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-error", "check")
	f.save(t, "b-current", "check")
	perTargetChecks(f, map[string]*projectDTO.UpdateInfo{"a-error": {Status: "error", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/app:1.0.0"}}}, "b-current": upToDate()})
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)
	require.Equal(t, st.Partial, out.Status)
	for _, target := range out.Targets {
		if target.TargetID == "a-error" {
			require.Equal(t, st.Failed, target.Status)
			require.Equal(t, "update.check-unavailable", target.Code)
		}
	}
}

func TestHostCheckWithoutAnySuccessFails(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-error", "check")
	f.save(t, "b-error", "check")
	failed := &projectDTO.UpdateInfo{Status: "error", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/app:1.0.0"}}}
	perTargetChecks(f, map[string]*projectDTO.UpdateInfo{"a-error": failed, "b-error": failed})
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)
	require.Equal(t, st.Failed, out.Status)
}

func TestHostCheckWithOnlyMissingImagesIsSkipped(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a-missing", "check")
	perTargetChecks(f, map[string]*projectDTO.UpdateInfo{"a-missing": {Status: "not_pulled", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"app": {ImageRef: "registry.example.com/missing:1.0.0"}}}})
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)
	require.Equal(t, st.Skipped, out.Status)
}

func TestUndiscoverableHostPlansEveryRetainedPolicy(t *testing.T) {
	f := newPolicyFixture(t)
	f.save(t, "a", "check")
	f.save(t, "b", "check")
	f.p.ports.host = func(context.Context, string) (string, error) { return "", errors.New("synthetic unreachable host") }
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)
	require.Equal(t, st.Failed, out.Status)
	require.Len(t, plans, 1)
	require.ElementsMatch(t, []string{"0~stack~a", "0~stack~b"}, plans[0].ids)
	counts, ok := persisted.Counts()
	require.True(t, ok)
	require.Equal(t, st.BulkCounts{Planned: 2, PlannedKnown: true, Completed: 2, Failed: 2}, counts)
}

func TestAllHostPlanIsKnownOnlyAfterTheLastHost(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Exec("INSERT INTO environments (id,name,api_url,enabled) VALUES (?,?,?,?)", "9-second", "Second", "", true).Error)
	f.save(t, "a", "check")
	f.p.ports.targets = func(_ context.Context, env string) ([]updateTarget, error) {
		if env == "0" {
			return []updateTarget{{Kind: "stack", ID: "a", Name: "Alpha", Eligible: true}}, nil
		}
		return []updateTarget{{Kind: "container", ID: "web", Name: "web", Eligible: true}, {Kind: "container", ID: "skip", Name: "skip"}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) { return upToDate(), nil }
	perTargetChecks(f, map[string]*projectDTO.UpdateInfo{"a": upToDate()})
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheck(progressContext(t, &plans, &persisted))
	require.NoError(t, err)
	require.Equal(t, st.Succeeded, out.Status)
	require.Equal(t, []planCall{
		{ids: []string{"0~stack~a"}, known: false},
		{ids: []string{"0~stack~a", "9-second~container~web"}, known: true},
	}, plans)
	require.Equal(t, "Alpha", out.Targets[0].Name)
}

func TestAllHostPlanWithoutHostsIsKnownEmpty(t *testing.T) {
	f := newPolicyFixture(t)
	require.NoError(t, f.db.Exec("UPDATE environments SET enabled = ?", false).Error)
	var plans []planCall
	var persisted st.Outcome
	_, err := f.s.RunUpdateCheck(progressContext(t, &plans, &persisted))
	require.NoError(t, err)
	require.Len(t, plans, 1)
	require.Empty(t, plans[0].ids)
	require.True(t, plans[0].known)
}

func TestContainerCheckSkipsMissingLocalImage(t *testing.T) {
	f := newPolicyFixture(t)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "container", ID: "web", Name: "web", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) {
		return &projectDTO.UpdateInfo{Status: "not_pulled", ImageCount: 1, ServiceUpdates: map[string]projectDTO.ServiceUpdateInfo{"web": {ImageRef: "registry.example.com/missing:1.0.0"}}}, nil
	}
	var plans []planCall
	var persisted st.Outcome
	out, err := f.s.RunUpdateCheckHost(progressContext(t, &plans, &persisted), "0")
	require.NoError(t, err)
	require.Len(t, out.Targets, 1)
	require.Equal(t, st.Skipped, out.Targets[0].Status)
	require.Equal(t, "update.image-not-pulled", out.Targets[0].Code)
	require.Equal(t, st.Skipped, out.Status)
}
