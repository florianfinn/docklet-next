// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original v2 window, trusted-check and policy identity regression coverage.

package updater

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humaecho"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/jobcontext"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/labstack/echo/v5"
	"github.com/stretchr/testify/require"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// Existing serial-child regression scenarios now fire the corresponding distinct
// window jobs. The production scheduler owns their individual durable records.
func runFixtureWindows(s *UpdaterService, ctx context.Context, env, job string) (outcome st.Outcome, err error) {
	defer func() {
		for i := range outcome.Targets {
			if outcome.Targets[i].TargetID != "" {
				outcome.Targets[i].ID = outcome.Targets[i].TargetID
			}
		}
	}()
	p := s.policies
	if job == "image-polling" {
		return s.runUpdateCheckHost(ctx, p, env)
	}
	var rows []policy.Policy
	if e := p.store.DB.Where("environment_id = ? AND registration_id = ?", env, "example-registration").Order("target_id").Find(&rows).Error; e != nil {
		return st.Outcome{}, e
	}
	run, _ := jobcontext.Run(ctx)
	if run.Trigger == "recovery" || run.AttemptCount > 1 || !p.startedAt.IsZero() && run.CreatedAt.Before(p.startedAt) {
		return s.ReconcileImagePolicyHost(ctx, env, run)
	}
	out := st.Outcome{Status: st.Succeeded}
	for _, row := range rows {
		if e := ctx.Err(); e != nil {
			return out, e
		}
		one, e := s.RunUpdateWindow(ctx, row.ID)
		if len(one.Targets) > 0 && len(out.Targets) > 0 && one.Targets[0].ActivityID != "" && one.Targets[0].ActivityID == out.Targets[len(out.Targets)-1].ActivityID {
			return out, e
		}
		out.Targets = append(out.Targets, one.Targets...)
		if e != nil {
			return out, e
		}
		if one.Code == "runtime.outcome-unknown" {
			out.Code = one.Code
			out.Status = st.Failed
			break
		}
	}
	return policyResult(out), nil
}

func TestUpdateWindowOnlyScheduledAndActorRevalidated(t *testing.T) {
	for _, mode := range []string{"check", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			f := newPolicyFixture(t)
			one := f.save(t, "app", mode)
			ctx := jobcontext.WithExecution(t.Context(), st.Run{ID: "window-run", AttemptCount: 1}, nil)
			_, err := f.s.RunUpdateWindow(ctx, one.ID)
			require.NoError(t, err)
			if mode == "check" {
				require.Zero(t, f.admissions)
				require.Zero(t, f.checks)
				return
			}
			require.Equal(t, 1, f.admissions)
			f.revoked = true
			out, err := f.s.RunUpdateWindow(jobcontext.WithExecution(t.Context(), st.Run{ID: "next-window", AttemptCount: 1}, nil), one.ID)
			require.NoError(t, err)
			require.Equal(t, "update.permission-denied", out.Targets[0].Code)
			require.Equal(t, 1, f.admissions)
		})
	}
}

func TestUpdateCheckDefaultAndTrustedObservation(t *testing.T) {
	f := newPolicyFixture(t)
	f.revoked = true
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) {
		return []updateTarget{{Kind: "stack", ID: "new-stack", Name: "new-stack", Eligible: true}, {Kind: "container", ID: "app", Name: "app", ContainerID: "first-id", Eligible: true}}, nil
	}
	f.p.ports.checkContainer = func(context.Context, string, updateTarget) (*projectDTO.UpdateInfo, error) {
		f.checks++
		return f.check, nil
	}
	out, err := f.s.RunUpdateCheck(t.Context())
	require.NoError(t, err)
	require.Len(t, out.Targets, 2)
	require.Equal(t, 2, f.checks)
	require.Zero(t, f.actors)
	require.Zero(t, f.admissions)
	rows, err := f.s.ListImagePolicies(t.Context(), "0")
	require.NoError(t, err)
	require.Len(t, rows, 2)
	for _, row := range rows {
		require.Equal(t, "check", row.Mode)
		require.EqualValues(t, 1, row.Revision)
		require.NotNil(t, row.CheckedAt)
	}
}

func TestDefaultStackPolicyProjectsAvailableIdentityWithoutRequiringBaseline(t *testing.T) {
	f := newPolicyFixture(t)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "new-stack", "stack")
	require.NoError(t, err)
	require.Equal(t, f.baseline.TargetIdentity, row.TargetIdentity)
	require.Zero(t, row.Revision)
	f.p.ports.baseline = func(context.Context, string, string) (dto.BaselineStatus, error) {
		return dto.BaselineStatus{}, imageDTO.Failure("update.baseline-unavailable", 409)
	}
	row, err = f.s.GetImagePolicy(t.Context(), "0", "without-baseline", "stack")
	require.NoError(t, err)
	require.Equal(t, "check", row.Mode)
	require.Empty(t, row.TargetIdentity)
	require.Zero(t, row.Revision)
}

func TestRemoveMissingScheduledPolicyPreservesPruningClock(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "missing", "scheduled")
	since := time.Now().UTC().Add(-7 * 24 * time.Hour)
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Update("missing_since", since).Error)
	jobs := map[string]bool{UpdateWindowJobKey("0", "stack", "missing"): true}
	f.p.reschedule = func(ctx context.Context) error {
		rows, e := f.s.ScheduledUpdatePolicies(ctx)
		if e != nil {
			return e
		}
		clear(jobs)
		for _, r := range rows {
			jobs[UpdateWindowJobKey(r.EnvironmentID, r.TargetKind, r.TargetID)] = true
		}
		return nil
	}
	router := echo.New()
	api := humaecho.New(router, huma.DefaultConfig("remove-policy-test", "v14"))
	registerImagePolicyRoutes(api, f.s)
	request := func(method, path, body string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(method, path, strings.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		ctx := context.WithValue(req.Context(), middleware.ContextKeyUserPermissions, authz.SudoPermissionSet())
		ctx = context.WithValue(ctx, middleware.ContextKeyUserID, "example-actor")
		response := httptest.NewRecorder()
		router.ServeHTTP(response, req.WithContext(ctx))
		require.Equal(t, http.StatusOK, response.Code, response.Body.String())
		return response
	}
	updated := request(http.MethodPut, "/environments/0/updater/policies/stack/missing", `{"mode":"check","expectedRevision":1}`)
	require.Contains(t, updated.Body.String(), `"mode":"check"`)
	require.Empty(t, jobs)
	require.NoError(t, f.db.First(&one, "id = ?", one.ID).Error)
	require.NotNil(t, one.MissingSince)
	require.WithinDuration(t, since, *one.MissingSince, time.Millisecond)
	require.NoError(t, f.p.store.PruneAbsent(t.Context(), "0", "example-registration", map[string]bool{}, since.Add(7*24*time.Hour-time.Second)))
	var n int64
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Count(&n).Error)
	require.EqualValues(t, 1, n)
	f.p.ports.targets = func(context.Context, string) ([]updateTarget, error) { return []updateTarget{}, nil }
	request(http.MethodGet, "/environments/0/updater/policies", "")
	require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Count(&n).Error)
	require.Zero(t, n)
}
