// Source: docklet-next/docs/design/update-policies.md
// Revision: 8907719aba6cd2d3037d287729f689d5ad9e3ec9
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Runtime reason rejection, polling freshness and durable host permission refusals.

package updater

import (
	"context"
	"errors"
	"fmt"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	policy "github.com/florianfinn/docklet-next/backend/internal/updater/children/policies"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	projectDTO "github.com/florianfinn/docklet-next/types/project"
	st "github.com/florianfinn/docklet-next/types/scheduler"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestPolicyCodePermissionAndDeclaredReasons(t *testing.T) {
	for _, tc := range []struct {
		name string
		err  error
		want string
	}{
		{"unauthorized", huma.Error401Unauthorized("Synthetic unauthenticated actor"), "update.permission-denied"},
		{"forbidden", huma.Error403Forbidden("Synthetic revoked actor"), "update.permission-denied"},
		{"wrapped-forbidden", fmt.Errorf("synthetic wrapper: %w", huma.Error403Forbidden("Synthetic revoked actor")), "update.permission-denied"},
		{"bad-request", huma.Error400BadRequest("Synthetic bad input"), "update.persistence-failed"},
		{"nil", nil, "update.persistence-failed"},
		{"generic", errors.New("Synthetic failure"), "update.persistence-failed"},
		{"typed", imageDTO.Failure("update.config-drift", 409), "update.config-drift"},
		{"unknown-typed", imageDTO.Failure("FORBIDDEN", 403), "update.persistence-failed"},
		{"gate", &activity.GateConflict{Code: "run.busy"}, "run.busy"},
		{"unknown-gate", &activity.GateConflict{Code: "BOGUS"}, "update.persistence-failed"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := policyCode(tc.err, "update.persistence-failed")
			require.Equal(t, tc.want, got)
			require.True(t, imageDTO.ValidReason(got))
		})
	}
}

func TestPolicyPollingCheckedAtRequiresSuccessfulCheck(t *testing.T) {
	for _, previous := range []bool{false, true} {
		for _, result := range []string{"success", "failed", "skipped"} {
			t.Run(fmt.Sprintf("previous=%v/%s", previous, result), func(t *testing.T) {
				f := newPolicyFixture(t)
				one := f.save(t, "stack", "apply")
				var expected *time.Time
				if previous {
					old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
					expected = &old
					require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Update("checked_at", old).Error)
				}
				if result == "failed" {
					f.revoked = true
				}
				if result == "skipped" {
					f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
						return nil, errors.New("Synthetic check unavailable")
					}
				}
				out := f.run(t, "image-polling", st.Run{})
				row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
				require.NoError(t, err)
				if result == "success" {
					require.Equal(t, st.Succeeded, out.Targets[0].Status)
					require.NotNil(t, row.CheckedAt)
					if expected != nil {
						require.True(t, row.CheckedAt.After(*expected))
					}
				} else {
					require.NotEqual(t, st.Succeeded, out.Targets[0].Status)
					require.Equal(t, expected, row.CheckedAt)
					require.NotEmpty(t, row.Code)
				}
				require.Nil(t, row.ApplyCheckedAt)
				require.Zero(t, f.admissions)
			})
		}
	}
}

func TestPolicyHostPermissionRefusalsPersistApplyStatus(t *testing.T) {
	for _, boundary := range []string{"owner", "before-target", "later-target", "missing-port"} {
		t.Run(boundary, func(t *testing.T) {
			f := newPolicyFixture(t)
			for _, target := range []string{"a-stack", "b-stack"} {
				f.save(t, target, "apply")
			}
			f.save(t, "c-notify", "notify")
			f.save(t, "d-off", "off")
			old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
			require.NoError(t, f.db.Model(&policy.Policy{}).Where("environment_id = ?", "0").Updates(map[string]any{"checked_at": old, "bookkeeping_code": "update.check-unavailable", "bookkeeping_at": old}).Error)
			foreign := f.save(t, "foreign", "apply")
			require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", foreign.ID).Update("registration_id", "another-registration").Error)
			if boundary == "missing-port" {
				f.p.ports.scheduleActor = nil
			} else {
				calls := 0
				limit := map[string]int{"owner": 0, "before-target": 1, "later-target": 2}[boundary]
				f.p.ports.scheduleActor = func(context.Context, string, string, string) error {
					calls++
					if calls > limit {
						return huma.Error403Forbidden("Synthetic owner revoked")
					}
					return nil
				}
			}
			out := f.run(t, "auto-update", st.Run{})
			require.Contains(t, []st.RunStatus{st.Failed, st.Partial}, out.Status)
			for _, target := range []string{"a-stack", "b-stack"} {
				row, err := f.s.GetImagePolicy(t.Context(), "0", target)
				require.NoError(t, err)
				if boundary == "later-target" && target == "a-stack" {
					require.Empty(t, row.ApplyCode)
				} else {
					require.Equal(t, "update.permission-denied", row.ApplyCode)
				}
				require.NotNil(t, row.ApplyCheckedAt)
				require.Empty(t, row.Code)
				require.Equal(t, &old, row.CheckedAt)
				require.Equal(t, "update.check-unavailable", row.BookkeepingCode)
				require.Equal(t, &old, row.BookkeepingAt)
			}
			for _, target := range []string{"c-notify", "d-off"} {
				row, err := f.s.GetImagePolicy(t.Context(), "0", target)
				require.NoError(t, err)
				require.Empty(t, row.ApplyCode)
				require.Nil(t, row.ApplyCheckedAt)
				require.Equal(t, &old, row.CheckedAt)
			}
			var unrelated policy.Policy
			require.NoError(t, f.db.First(&unrelated, "id = ?", foreign.ID).Error)
			require.Empty(t, unrelated.ApplyCode)
			require.Nil(t, unrelated.ApplyCheckedAt)
			expected := 0
			if boundary == "later-target" {
				expected = 1
			}
			require.Equal(t, expected, f.admissions)
			require.Equal(t, expected, f.checks)
		})
	}
}

func TestPolicyUnknownProducerReasonNeverReachesStatus(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "stack", "apply")
	f.baseline.Available = false
	f.baseline.Code = "FORBIDDEN"
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "update.persistence-failed", out.Targets[0].Code)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Empty(t, row.ApplyCode)
	require.Nil(t, row.ApplyCheckedAt)
	require.Nil(t, row.CheckedAt)
	var children int64
	require.NoError(t, f.db.Model(&policy.Child{}).Where("policy_id = ?", one.ID).Count(&children).Error)
	require.Zero(t, children)
	require.Zero(t, f.admissions)
}

func TestPolicyHostRefusalStatusFailureAndRevisionRace(t *testing.T) {
	for _, boundary := range []string{"owner", "before-target"} {
		for _, race := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/revision=%v", boundary, race), func(t *testing.T) {
				f := newPolicyFixture(t)
				one := f.save(t, "stack", "apply")
				if !race {
					require.NoError(t, f.db.Exec("CREATE TRIGGER synthetic_refusal_write_failure BEFORE UPDATE ON stack_update_policies WHEN NEW.apply_code='update.permission-denied' BEGIN SELECT RAISE(ABORT,'synthetic status failure'); END").Error)
				}
				calls := 0
				limit := 0
				if boundary == "before-target" {
					limit = 1
				}
				f.p.ports.scheduleActor = func(context.Context, string, string, string) error {
					calls++
					if calls <= limit {
						return nil
					}
					if race {
						f.save(t, "stack", "off")
					}
					return huma.Error403Forbidden("Synthetic permission refusal")
				}
				out := f.run(t, "auto-update", st.Run{})
				require.Len(t, out.Targets, 1)
				expected := "update.persistence-failed"
				if race {
					expected = "update.permission-denied"
				}
				require.Equal(t, expected, out.Targets[0].Code)
				var stored policy.Policy
				require.NoError(t, f.db.First(&stored, "id = ?", one.ID).Error)
				require.Empty(t, stored.ApplyCode)
				require.Nil(t, stored.ApplyCheckedAt)
				require.Nil(t, stored.CheckedAt)
				if race {
					require.EqualValues(t, 2, stored.Revision)
					require.Equal(t, "off", stored.Mode)
				}
				require.Zero(t, f.checks)
				require.Zero(t, f.admissions)
			})
		}
	}
}
