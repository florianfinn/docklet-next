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
				one := f.save(t, "stack", "scheduled")
				var expected *time.Time
				if previous {
					old := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
					expected = &old
					require.NoError(t, f.db.Model(&policy.Policy{}).Where("id = ?", one.ID).Update("checked_at", old).Error)
				}
				if result == "failed" {
					f.p.ports.check = func(context.Context, string, string) (*projectDTO.UpdateInfo, error) {
						return nil, errors.New("synthetic failed observation")
					}
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

func TestPolicyUnknownProducerReasonNeverReachesStatus(t *testing.T) {
	f := newPolicyFixture(t)
	one := f.save(t, "stack", "scheduled")
	f.baseline.Available = false
	f.baseline.Code = "FORBIDDEN"
	out := f.run(t, "auto-update", st.Run{})
	require.Equal(t, "update.persistence-failed", out.Targets[0].Code)
	row, err := f.s.GetImagePolicy(t.Context(), "0", "stack")
	require.NoError(t, err)
	require.Equal(t, "update.persistence-failed", row.ApplyCode)
	require.NotNil(t, row.ApplyCheckedAt)
	require.Nil(t, row.CheckedAt)
	var children int64
	require.NoError(t, f.db.Model(&policy.Child{}).Where("policy_id = ?", one.ID).Count(&children).Error)
	require.Zero(t, children)
	require.Zero(t, f.admissions)
}
