// Source: docklet-next/issues/289
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backuptarget

import (
	"context"
	"encoding/json"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"gorm.io/gorm"
	"slices"
	"time"
)

type deliveryAttempt struct {
	Authority            string    `json:"authority"`
	RegistrationRevision int64     `json:"registrationRevision"`
	SetVersion           int64     `json:"setVersion"`
	Outcome              string    `json:"outcome"`
	At                   time.Time `json:"at"`
}

// The receipt stays at the top level for existing stored-receipt readers.
type deliveryRecord struct {
	dto.TargetSetReceipt
	RegistrationRevision int64            `json:"registrationRevision"`
	Targets              map[string]int64 `json:"targets"`
	At                   time.Time        `json:"at"`
	LastAttempt          *deliveryAttempt `json:"lastAttempt,omitempty"`
}

func registrationRevision(tx *gorm.DB, host string) (int64, error) {
	// Lightweight stores without host registrations still support target metadata delivery.
	if !tx.Migrator().HasTable("environments") {
		return 0, nil
	}
	var row struct{ CredentialRevision int64 }
	result := tx.Table("environments").Select("credential_revision").Where("id = ?", host).Limit(1).Find(&row)
	if result.Error != nil {
		return 0, result.Error
	}
	if result.RowsAffected == 0 {
		return 0, gorm.ErrRecordNotFound
	}
	return row.CredentialRevision, nil
}
func saveDelivery(ctx context.Context, s *Store, host string, set dto.TargetSet, revision int64, receipt *dto.TargetSetReceipt, outcome string, at time.Time) error {
	return s.DB.WithContext(context.WithoutCancel(ctx)).Transaction(func(tx *gorm.DB) error {
		current, err := registrationRevision(tx, host)
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return nil
		}
		if err != nil {
			return err
		}
		authority, err := readSetting(tx, "backup_target_authority")
		if err != nil {
			return err
		}
		if current != revision || authority != set.Authority {
			return nil
		}
		text, err := readSetting(tx, "backupTargetDelivery:"+host)
		if err != nil {
			return err
		}
		var held deliveryRecord
		if text != "" {
			if err = json.Unmarshal([]byte(text), &held); err != nil {
				return err
			}
		}
		if held.Authority != set.Authority || held.RegistrationRevision != revision {
			held = deliveryRecord{TargetSetReceipt: dto.TargetSetReceipt{Authority: set.Authority}, RegistrationRevision: revision}
		}
		if held.LastAttempt == nil || held.LastAttempt.SetVersion <= set.SetVersion && !held.LastAttempt.At.After(at) {
			held.LastAttempt = &deliveryAttempt{Authority: set.Authority, RegistrationRevision: revision, SetVersion: set.SetVersion, Outcome: outcome, At: at}
		}
		if receipt != nil && (held.Authority == "" || receipt.SetVersion >= held.SetVersion) {
			held.TargetSetReceipt = *receipt
			held.RegistrationRevision = revision
			held.At = at
			held.Targets = map[string]int64{}
			for _, target := range set.Targets {
				if !slices.Contains(receipt.Conflicts, target.ID) {
					held.Targets[target.ID] = target.ConfigVersion
				}
			}
		}
		encoded, err := json.Marshal(held)
		if err != nil {
			return err
		}
		return writeSetting(tx, "backupTargetDelivery:"+host, string(encoded))
	})
}
func projectDelivery(record deliveryRecord, authority string, revision, version int64, target Target, host string) dto.TargetHostDelivery {
	result := dto.TargetHostDelivery{HostID: host, State: "pending"}
	receiptCurrent := record.Authority == authority && record.RegistrationRevision == revision
	attempt := record.LastAttempt
	if attempt != nil && (attempt.Authority != authority || attempt.RegistrationRevision != revision) {
		attempt = nil
	}
	if receiptCurrent && slices.Contains(record.Conflicts, target.ID) {
		result.State = "conflict"
		result.Code = "backup.target.location-conflict"
		result.At = &record.At
		return result
	}
	if attempt != nil && attempt.SetVersion == version && (attempt.Outcome == "backup.target.stale-set" || attempt.Outcome == "backup.target.wrong-host") {
		result.State = "conflict"
		result.Code = attempt.Outcome
		result.At = &attempt.At
		return result
	}
	if receiptCurrent && record.Targets[target.ID] == target.ConfigVersion {
		result.State = "current"
		result.At = &record.At
		return result
	}
	if attempt != nil && attempt.Outcome != "ok" {
		result.Code = attempt.Outcome
		result.At = &attempt.At
	}
	return result
}
func (s *Service) deliveries(ctx context.Context, target Target) ([]dto.TargetHostDelivery, error) {
	result := []dto.TargetHostDelivery{}
	if !s.deps.Store.DB.Migrator().HasTable("environments") {
		return result, nil
	}
	authority, version, err := s.deps.Store.Metadata(ctx, false)
	if err != nil {
		return nil, err
	}
	var hosts []struct {
		ID                 string
		CredentialRevision int64
		Enabled            bool
	}
	if err = s.deps.Store.DB.WithContext(ctx).Table("environments").Where("enabled = ?", true).Order("id").Find(&hosts).Error; err != nil {
		return nil, err
	}
	for _, host := range hosts {
		if target.HostID != nil && *target.HostID != host.ID {
			continue
		}
		if host.ID == "0" {
			result = append(result, dto.TargetHostDelivery{HostID: "0", State: "current"})
			continue
		}
		text, err := readSetting(s.deps.Store.DB.WithContext(ctx), "backupTargetDelivery:"+host.ID)
		if err != nil {
			return nil, err
		}
		var record deliveryRecord
		if text != "" {
			if err = json.Unmarshal([]byte(text), &record); err != nil {
				return nil, err
			}
		}
		result = append(result, projectDelivery(record, authority, host.CredentialRevision, version, target, host.ID))
	}
	return result, nil
}
