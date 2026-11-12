// Source: docklet-next/issues/439
// Revision: 910032a54830f5ce1d37070964fa2dc7efb8aa2f
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original Compose-faithful self-update implementation.

package selfupdate

import (
	dto "github.com/florianfinn/docklet-next/types/environment"
	"gorm.io/gorm"
)

// CheckSelfUpdateHostTx is shared by restart, runtime and backup admission.
// The durable KV owner outlives the original hub/agent process and HTTP request.
func CheckHostTx(tx *gorm.DB, environment string) error {
	if !tx.Migrator().HasTable("kv") {
		return nil
	}
	var n int64
	err := tx.Table("kv").Where("(key = ? OR key LIKE ?) AND json_extract(value, '$.environmentId') = ? AND json_extract(value, '$.phase') <> ?", "selfupdate.operation", "selfupdate.remote.%", environment, "settled").Count(&n).Error
	if err != nil {
		return err
	}
	if n > 0 {
		return &dto.HostProblem{Code: "host.busy", Status: 409}
	}
	return nil
}
