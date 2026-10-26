// Source: docklet-next/docs/design/update-policies.md
// Revision: 839b98c10360da9b7beada6804d70d3da98ac8ed
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Scoped update capability and restore-point projection implementation for #229.

package updater

import (
	"encoding/json"
	jsonv2 "encoding/json/v2"
	"errors"
	"github.com/florianfinn/docklet-next/types/image"
)

type RestorePointProtectionReason string

const (
	RestoreTarget   RestorePointProtectionReason = "restore-target"
	HealthyBaseline RestorePointProtectionReason = "healthy-baseline"
)

type RestorePointHoldState string

const (
	HoldNone    RestorePointHoldState = "none"
	HoldActive  RestorePointHoldState = "active"
	HoldUnknown RestorePointHoldState = "unknown"
)
const MaxRestorePointResponseBytes = 8 << 20
const MaxRestorePointResponseCount = 4096

func MergePointHold(a, b RestorePointHoldState) RestorePointHoldState {
	if a == HoldUnknown || b == HoldUnknown {
		return HoldUnknown
	}
	if a == HoldActive || b == HoldActive {
		return HoldActive
	}
	return HoldNone
}

// UnmarshalJSON enforces required fields even when their zero value is valid.
func (p *RestorePoint) UnmarshalJSON(body []byte) error {
	type plain RestorePoint
	var value plain
	if err := jsonv2.Unmarshal(body, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := jsonv2.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, key := range []string{"protectionReason", "holdState", "failedAttempt", "id", "registrationId", "targetId", "targetIdentity", "sourceRunId", "createdAt", "sourceState", "images", "volumeDataIncluded", "available", "orphaned"} {
		raw, ok := fields[key]
		if !ok || (key != "protectionReason" && string(raw) == "null") {
			return errors.New("invalid restore point fields")
		}
	}
	if value.ProtectionReason != nil && *value.ProtectionReason != RestoreTarget && *value.ProtectionReason != HealthyBaseline {
		return errors.New("invalid restore point protection")
	}
	if value.HoldState != HoldNone && value.HoldState != HoldActive && value.HoldState != HoldUnknown {
		return errors.New("invalid restore point hold")
	}
	if value.ID == "" || value.RegistrationID == "" || value.TargetID == "" || value.TargetIdentity == "" || value.SourceRunID == "" || value.CreatedAt.IsZero() || (value.SourceState != "settled" && value.SourceState != "partial-source") || value.Images == nil || image.ValidateReason(value.Code) != nil {
		return errors.New("invalid restore point metadata")
	}
	*p = RestorePoint(value)
	return nil
}

func (p *PointImage) UnmarshalJSON(body []byte) error {
	type plain PointImage
	var value plain
	if err := jsonv2.Unmarshal(body, &value); err != nil {
		return err
	}
	var fields map[string]json.RawMessage
	if err := jsonv2.Unmarshal(body, &fields); err != nil {
		return err
	}
	for _, key := range []string{"service", "reference", "imageId", "present", "running"} {
		raw, ok := fields[key]
		if !ok || string(raw) == "null" {
			return errors.New("invalid restore point image")
		}
	}
	*p = PointImage(value)
	return nil
}
