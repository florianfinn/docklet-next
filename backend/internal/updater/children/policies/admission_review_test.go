// Source: docklet-next/docs/design/update-policies.md
// Revision: 4023414fcf620a9ea4be2429ee70fa8fd8230f9c
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: One-at-a-time persisted admission binding changes isolate each barrier.

package policies

import (
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"testing"
)

func TestEveryPolicyAdmissionBindingIsRequired(t *testing.T) {
	for _, boundary := range []string{"missing-policy", "missing-child", "environment", "registration", "target", "identity", "mode", "policy-revision", "actor", "child-policy", "child-revision", "child-job", "child-status"} {
		t.Run(boundary, func(t *testing.T) {
			f := newStoreFixture(t)
			p := f.policy(t, "apply")
			child, _, err := f.PrepareChild(t.Context(), p, "example-job")
			require.NoError(t, err)
			b := &dto.PolicyBinding{PolicyID: p.ID, Revision: p.Revision, JobRunID: child.JobRunID, ChildID: child.ID}
			validate := func() error {
				return ValidateAdmission(f.DB, b, p.EnvironmentID, p.RegistrationID, p.TargetID, p.TargetIdentity, p.ActorID)
			}
			require.NoError(t, validate())
			switch boundary {
			case "missing-policy":
				require.NoError(t, f.DB.Delete(&Policy{}, "id = ?", p.ID).Error)
			case "missing-child":
				require.NoError(t, f.DB.Delete(&Child{}, "id = ?", child.ID).Error)
			case "environment":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("environment_id", "replacement").Error)
			case "registration":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("registration_id", "replacement").Error)
			case "target":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("target_id", "replacement").Error)
			case "identity":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("target_identity", "replacement").Error)
			case "mode":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("mode", "off").Error)
			case "policy-revision":
				b.Revision++
			case "actor":
				require.NoError(t, f.DB.Model(&Policy{}).Where("id = ?", p.ID).Update("actor_id", "replacement").Error)
			case "child-policy":
				require.NoError(t, f.DB.Model(&Child{}).Where("id = ?", child.ID).Update("policy_id", "replacement").Error)
			case "child-revision":
				require.NoError(t, f.DB.Model(&Child{}).Where("id = ?", child.ID).Update("revision", child.Revision+1).Error)
			case "child-job":
				require.NoError(t, f.DB.Model(&Child{}).Where("id = ?", child.ID).Update("job_run_id", "replacement").Error)
			case "child-status":
				require.NoError(t, f.DB.Model(&Child{}).Where("id = ?", child.ID).Update("status", "settled").Error)
			}
			require.Equal(t, "update.policy-changed", imageDTO.Neutral(validate(), "unexpected", 500).Problem.Code)
		})
	}
}
