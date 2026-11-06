// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; write validation for upgrade retention and features. #69 first-run setup and hub settings persistence; M14 container and host operation contracts; M16 bind mounts and backup completion; M17 runtime history and personal preferences; M18 live scheduling, policy and contract foundation; M19 backend contracts and target identity integration.

package settings

import (
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/types/features"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/settings"
)

func validateSettingValueInternal(key, value string) error {
	if key == "activityEventLimit" || key == "activityRunLimit" {
		low, high := 50, 5000
		if key == "activityRunLimit" {
			low, high = 10, 1000
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < low || n > high {
			return &dto.Problem{Code: "settings.activity-limit-invalid", Field: "/" + key, Status: 400}
		}
	}

	if key == "updateCheckInterval" && value != "6h" && value != "12h" && value != "24h" {
		return common.Classify(common.ErrValidation, errors.New("updateCheckInterval must be 6h, 12h or 24h"))
	}
	if key == "updateNotify" && value != "true" && value != "false" {
		return common.Classify(common.ErrValidation, errors.New("updateNotify must be true or false"))
	}
	if key == "timeZone" && value != "" {
		// Local and Factory load but name no IANA zone that clients can format with.
		if _, err := time.LoadLocation(value); err != nil || value == "Local" || value == "Factory" {
			return imageDTO.Failure("settings.invalid-time-zone", 400)
		}
	}
	if key == "loadAlertsEnabled" && value != "true" && value != "false" {
		return common.Classify(common.ErrValidation, errors.New("loadAlertsEnabled must be true or false"))
	}
	if key == "loadAlertCpuPercent" || key == "loadAlertMemoryPercent" || key == "loadAlertMinutes" {
		minimum, maximum := 50, 100
		if key == "loadAlertMinutes" {
			minimum, maximum = 5, 60
		}
		n, err := strconv.Atoi(value)
		if err != nil || n < minimum || n > maximum {
			return common.Classify(common.ErrValidation, fmt.Errorf("%s must be a whole number between %d and %d", key, minimum, maximum))
		}
	}
	if key == "stackStopMode" && value != "down" && value != "stop" {
		return common.Classify(common.ErrValidation, errors.New("stackStopMode must be down or stop"))
	}
	if key == "vulnerabilityScanSchedule" {
		if _, err := dto.ParseVulnerabilitySchedule(value); err != nil {
			return common.Classify(common.ErrValidation, err)
		}
	}
	switch key {
	case "trivyIgnoreUnfixed", "vulnerabilityNotifyCritical", "vulnerabilityNotifyHigh", "vulnerabilityNotifyFailed":
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
	if key == "backupDefaultRetentionCount" {
		count, err := strconv.Atoi(value)
		if err != nil || count < 0 || count > 3650 {
			return common.Classify(common.ErrValidation, errors.New("backupDefaultRetentionCount must be a whole number between 0 and 3650"))
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
