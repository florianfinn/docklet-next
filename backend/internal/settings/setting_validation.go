// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; write validation for upgrade retention and features. #69 first-run setup and hub settings persistence; M14 container and host operation contracts.

package settings

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/settings"
)

func validateSettingValueInternal(key, value string) error {
	if key == "stackStopMode" && value != "down" && value != "stop" {
		return common.Classify(common.ErrValidation, errors.New("stackStopMode must be down or stop"))
	}
	if key == "vulnerabilityScanSchedule" {
		if _, err := dto.ParseVulnerabilitySchedule(value); err != nil {
			return common.Classify(common.ErrValidation, err)
		}
	}
	switch key {
	case "vulnerabilityScanEnabled", "trivyIgnoreUnfixed", "vulnerabilityNotifyCritical", "vulnerabilityNotifyHigh", "vulnerabilityNotifyFailed":
		if value != "true" && value != "false" {
			return common.Classify(common.ErrValidation, errors.New("scan switch must be true or false"))
		}
	}

	if key == "toolsImage" && value != "" {
		if _, err := reference.ParseNormalizedNamed(value); err != nil {
			return common.Classify(common.ErrValidation, errors.New("toolsImage must be a valid Docker image reference or empty for the release default"))
		}
	}
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
	if key == "actionPresentation" && value != "dialog" && value != "toast" {
		return common.Classify(common.ErrValidation, errors.New("actionPresentation must be dialog or toast"))
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
