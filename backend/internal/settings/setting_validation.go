// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; write validation for upgrade retention and features. #69 first-run setup and hub settings persistence.

package settings

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/features"
)

func validateSettingValueInternal(key, value string) error {
	if key == "setupCompleted" || key == "detectSocketGroup" || key == "keepActionOutput" {
		if value != "true" && value != "false" {
			return common.Classify(common.ErrValidation, fmt.Errorf("%s must be true or false", key))
		}
	}
	if key == "linkBase" && value != "auto" && value != "internal" && value != "domain" {
		return common.Classify(common.ErrValidation, errors.New("linkBase must be auto, internal, or domain"))
	}
	if key == "defaultHostConnection" && value != "direct" && value != "edge" {
		return common.Classify(common.ErrValidation, errors.New("defaultHostConnection must be direct or edge"))
	}
	if key == "timeZone" && value != "" {
		if _, err := time.LoadLocation(value); err != nil {
			return common.Classify(common.ErrValidation, errors.New("timeZone must be an IANA time zone"))
		}
	}
	if key == "actionPresentation" && value != "dialog" && value != "toast" {
		return common.Classify(common.ErrValidation, errors.New("actionPresentation must be dialog or toast"))
	}
	if (key == "maxEvents" || key == "maxFinishedActions") && value != "" {
		limit, err := strconv.Atoi(value)
		if err != nil || limit < 1 || limit > 100000 {
			return common.Classify(common.ErrValidation, fmt.Errorf("%s must be between 1 and 100000", key))
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
