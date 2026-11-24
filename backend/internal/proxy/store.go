// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original engine-independent proxy contract foundation.

package proxy

import (
	"context"
	"encoding/json/v2"
	"errors"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"gorm.io/gorm"
	"strings"
	"time"
)

type Journal struct {
	ID           string `gorm:"primaryKey"`
	Source       string
	Generation   string
	Digest       string
	Registration string
	State        dto.State
	Phase        dto.Phase
	Intent       []byte
	Result       []byte
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

func (Journal) TableName() string { return "proxy_journal" }

type Record struct {
	ID           string `gorm:"primaryKey"`
	InstanceID   string
	Generation   string
	Digest       string
	Registration string
	State        dto.State
	Reason       dto.Reason
	Evidence     dto.EvidenceKind
	Actor        string
	Request      []byte
	Result       []byte
	CreatedAt    time.Time
	FinishedAt   *time.Time
}

func (Record) TableName() string { return "proxy_operations" }

type instanceRow struct {
	ID            string `gorm:"primaryKey"`
	EnvironmentID string
	Data          []byte
}

func (instanceRow) TableName() string { return "proxy_instances" }

type reservation struct {
	InstanceID  string `gorm:"primaryKey"`
	OperationID string
}

func (reservation) TableName() string { return "proxy_reservations" }

type admissionRow struct {
	ID           string `gorm:"primaryKey"`
	Generation   string
	Registration string
}

func (admissionRow) TableName() string { return "proxy_admissions" }

type fenceRow struct {
	ID           string `gorm:"primaryKey"`
	Generation   string
	Registration string
}

func (fenceRow) TableName() string { return "proxy_fences" }

type verifiedFile struct {
	Source  string `gorm:"primaryKey"`
	Path    string `gorm:"primaryKey"`
	Hash    string
	Content []byte
}

func (verifiedFile) TableName() string { return "proxy_verified_files" }
func marshal(v any) ([]byte, error)    { return json.Marshal(v) }
func decodeJournal(j Journal) (Intent, dto.Operation, error) {
	var i Intent
	o := journalReceipt(j)
	kind, valid := payloadKind(j.Intent)
	if valid && kind == dto.ResolverOperation {
		if binding, result, ok := decodeResolverPayload(j.Intent, j.Result, j.ID, j.Generation, j.Registration, j.Digest, j.State); ok {
			return Intent{Kind: kind, Request: binding}, result, nil
		}
	}
	if !valid || kind.Effective() != dto.RouteOperation {
		return i, deferredReceipt(o, kind), nil
	}
	if json.Unmarshal(j.Intent, &i) != nil || json.Unmarshal(j.Result, &o) != nil || validRequest(i.Request) != nil {
		return Intent{}, deferredReceipt(journalReceipt(j), kind), nil
	}
	if !j.State.Valid() || !j.Phase.Valid() || o.State != j.State || o.Phase != j.Phase || o.Digest != j.Digest {
		return i, o, dto.Fail(dto.JournalLost, 409)
	}
	if o.Kind.Effective() != dto.RouteOperation {
		return Intent{}, deferredReceipt(journalReceipt(j), o.Kind), nil
	}
	return i, o, nil
}
func decodeRecord(r Record) (dto.ApplyRequest, dto.Operation, error) {
	var q dto.ApplyRequest
	o := recordReceipt(r)
	kind, valid := payloadKind(r.Request)
	if valid && kind == dto.AdoptionOperation {
		if binding, result, ok := decodeAdoptionPayload(r); ok {
			return binding, result, nil
		}
	}

	if valid && kind == dto.ResolverOperation {
		if binding, result, ok := decodeResolverPayload(r.Request, r.Result, r.ID, r.Generation, r.Registration, r.Digest, r.State); ok {
			return binding, result, nil
		}
	}
	if !valid || kind.Effective() != dto.RouteOperation {
		// Decode only the shared executor binding; never interpret resolver data as a route.
		var binding struct {
			ID           string       `json:"id"`
			Instance     dto.Instance `json:"instance"`
			Generation   string       `json:"generation"`
			Registration string       `json:"registration"`
		}
		_ = json.Unmarshal(r.Request, &binding)
		q = dto.ApplyRequest{ID: binding.ID, Instance: binding.Instance, Generation: binding.Generation, Registration: binding.Registration}
		return q, deferredReceipt(o, kind), nil
	}
	if json.Unmarshal(r.Request, &q) != nil || json.Unmarshal(r.Result, &o) != nil || validRequest(q) != nil {
		return q, deferredReceipt(recordReceipt(r), kind), nil
	}
	if !r.State.Valid() || o.State != r.State {
		return q, o, dto.Fail(dto.JournalLost, 409)
	}
	if o.Kind.Effective() != dto.RouteOperation {
		return q, deferredReceipt(recordReceipt(r), o.Kind), nil
	}
	return q, o, nil
}
func unfinished(ctx context.Context, db *gorm.DB, table string) (string, error) {
	var id string
	err := db.WithContext(ctx).Table(table).Select("id").Where("finished_at IS NULL").Order("created_at").Limit(1).Scan(&id).Error
	return id, err
}
func prune(ctx context.Context, db *gorm.DB, now time.Time) error {
	for _, t := range []string{"proxy_operations", "proxy_journal"} {
		if err := db.WithContext(ctx).Table(t).Where("finished_at IS NOT NULL AND finished_at < ?", now.Add(-30*24*time.Hour)).Delete(map[string]any{}).Error; err != nil {
			return err
		}
	}
	return nil
}
func clearData(ctx context.Context, db *gorm.DB) (string, error) {
	var generation string
	err := db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, table := range []string{"proxy_reservations", "proxy_operations", "proxy_instances", "proxy_journal", "proxy_fences", "proxy_verified_files", "proxy_admissions"} {
			if err := tx.Exec("DELETE FROM " + table).Error; err != nil {
				return err
			}
		}
		if err := tx.Raw("SELECT lower(hex(randomblob(16)))").Scan(&generation).Error; err != nil {
			return err
		}
		result := tx.Table("proxy_generation").Where("id = 1").Update("generation", generation)
		if result.Error != nil {
			return result.Error
		}
		if result.RowsAffected != 1 {
			return dto.Fail(dto.JournalLost, 409)
		}
		return nil
	})
	return generation, err
}
func findJournal(ctx context.Context, db *gorm.DB, id string) (Journal, error) {
	var j Journal
	err := db.WithContext(ctx).First(&j, "id = ?", id).Error
	return j, err
}
func missing(err error) bool { return errors.Is(err, gorm.ErrRecordNotFound) }

type generationRow struct {
	ID         int `gorm:"primaryKey;autoIncrement:false"`
	Generation string
}

func (generationRow) TableName() string { return "proxy_generation" }

// Models extends the forward schema without changing the frozen 001 baseline.
func Models() []any {
	return []any{&generationRow{}, &instanceRow{}, &Record{}, &reservation{}, &Journal{}, &fenceRow{}, &verifiedFile{}, &admissionRow{}}
}

// Resolver plans own a project ancestor of their static/dynamic paths. Routes
// use the same overlap test, so direct host ingress cannot bypass this exclusion.
func lockSource(tx *gorm.DB, source string) error {
	var rows []Journal
	if err := tx.Select("id,source").Where("finished_at IS NULL").Find(&rows).Error; err != nil {
		return err
	}
	for _, j := range rows {
		if source == j.Source || strings.HasPrefix(source, j.Source+"/") || strings.HasPrefix(j.Source, source+"/") {
			return &dto.Problem{Status: 409, Code: dto.ModuleBusy, OperationID: j.ID}
		}
	}
	return nil
}
