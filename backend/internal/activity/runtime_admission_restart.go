// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original host restart exclusion for every runtime intake.

package activity

import (
	dto "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

// The caller owns the admission write transaction, also used by policy and
// scheduled child runs. A terminal restart releases this exclusion.
func runtimeCheckRestartTx(tx *gorm.DB, environment string) error {
	var active int64
	if err := tx.Table("agent_restart_requests").Where("environment_id = ? AND settled_at IS NULL", environment).Count(&active).Error; err != nil {
		return err
	}
	if active > 0 {
		return &dto.HostProblem{Code: "host.restarting", Status: 409}
	}
	return nil
}
