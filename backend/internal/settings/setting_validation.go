// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; bounded history-setting validation.

package settings

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/features"
)

func validateSettingValueInternal(key, value string) error {
	limits := map[string][2]int{
		"eventHistoryMaxEntries":    {50, 5000},
		"activityHistoryMaxEntries": {10, 1000},
	}
	if bounds, ok := limits[key]; ok && value != "" {
		count, err := strconv.Atoi(value)
		if err != nil || count < bounds[0] || count > bounds[1] {
			return common.Classify(common.ErrValidation, fmt.Errorf("%s must be a whole number between %d and %d", key, bounds[0], bounds[1]))
		}
	}
	if key == "upgradeLogRetentionDays" && value != "" {
		days, err := strconv.Atoi(value)
		if err != nil || days < 0 || days > 3650 {
			return common.Classify(common.ErrValidation, errors.New("upgradeLogRetentionDays must be a whole number between 0 and 3650"))
		}
	}
	for _, definition := range features.All() {
		if definition.SettingKey == key && value != "true" && value != "false" {
			return common.Classify(common.ErrValidation, fmt.Errorf("%s must be true or false", key))
		}
	}
	return nil
}
