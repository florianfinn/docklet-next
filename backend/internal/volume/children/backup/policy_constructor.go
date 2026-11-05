// Source: getarcaneapp/arcane/backend/internal/volume/children/backup/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed S3 backup destinations; M12 backup foundation; M12 persisted volume lifecycle and legacy restore cut.; Remove legacy archive and centralized volume execution; preserve historical metadata deletion and safety pins; notification and Auto Heal contracts and delivery for #296; M16 bind mounts and backup completion; M18 live scheduling, policy and contract foundation.

package backup

import "github.com/florianfinn/docklet-next/backend/internal/volume/children/backup/children/policies"

func (s *Service) newPolicies(deps Dependencies) *policies.Service {
	return policies.NewService(policies.Dependencies{
		DB:             deps.DB,
		ValidateTarget: s.validatePolicyTarget,
		Location:       deps.Location,
		ZoneProvider:   deps.ZoneProvider,
		Activity:       deps.Activity,
		AlreadyRunning: deps.AlreadyRunning,
		LatestRun:      s.latestRunInternal,
		CreateBackup:   s.scheduledBackupInternal,
		Reconcile:      s.ReconcileBackup,
	})
}
