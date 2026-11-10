// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package activity

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/selfupdate"

	dto "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

func CheckSelfUpdateHostTx(tx *gorm.DB, environment string) error {
	return selfupdate.CheckHostTx(tx, environment)
}

// CheckHostUpdateAdmissionTx refuses before reserving a self-update, under the
// same SQLite write transaction used by host restart and workload admission.
func CheckHostUpdateAdmissionTx(tx *gorm.DB, environment string) error {
	if err := CheckSelfUpdateHostTx(tx, environment); err != nil {
		return err
	}
	for _, check := range []struct {
		table, where string
		args         []any
	}{
		{"runtime_receipts", "environment_id = ? AND settled_at IS NULL", []any{environment}},
		{"agent_restart_requests", "environment_id = ? AND settled_at IS NULL", []any{environment}},
		{"activities", "environment_id = ? AND status IN ?", []any{environment, []string{"running", "queued"}}},
	} {
		if !tx.Migrator().HasTable(check.table) {
			continue
		}
		var n int64
		if err := tx.Table(check.table).Where(check.where, check.args...).Count(&n).Error; err != nil {
			return err
		}
		if n > 0 {
			return &dto.HostProblem{Code: "host.busy", Status: 409}
		}
	}
	return nil
}
func (s *ActivityService) CheckSelfUpdateHost(ctx context.Context, environment string) error {
	return CheckSelfUpdateHostTx(s.db.WithContext(ctx), environment)
}
