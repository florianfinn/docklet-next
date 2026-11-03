// Source: docklet-next/issues/339
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original per-user section replacement with transactional revision.

package user

import (
	"context"
	"encoding/json/v2"
	"errors"
	"log/slog"

	dto "github.com/florianfinn/docklet-next/types/preferences"
	"gorm.io/gorm"
)

func emptyUIPreferences() dto.Document { return dto.Document{Sections: map[string]map[string]any{}} }

func decodeUIPreferences(ctx context.Context, raw string) dto.Document {
	d := emptyUIPreferences()
	if raw == "{}" {
		return d
	}
	if err := json.Unmarshal([]byte(raw), &d); err != nil || d.Sections == nil || d.Revision < 0 {
		slog.WarnContext(ctx, "Stored UI preferences are invalid", "code", "preferences.invalid")
		return emptyUIPreferences()
	}
	return d
}

// The caller supplies only the authenticated identity; transport identities have no row.
func (s *UserService) UIPreferences(ctx context.Context, id string) (dto.Document, error) {
	var row struct{ UIPreferences string }
	result := s.db.WithContext(ctx).Table("users").Select("ui_preferences").Where("id=?", id).Take(&row)
	if errors.Is(result.Error, gorm.ErrRecordNotFound) {
		return dto.Document{}, &dto.Problem{Code: "preferences.no-user", Status: 403}
	}
	if result.Error != nil {
		return dto.Document{}, result.Error
	}
	return decodeUIPreferences(ctx, row.UIPreferences), nil
}

func (s *UserService) PutUIPreferences(ctx context.Context, id, section string, value any) (dto.Document, error) {
	if !dto.KnownSection(section) {
		return dto.Document{}, &dto.Problem{Code: "preferences.unknown-section", Status: 400}
	}
	object, ok := value.(map[string]any)
	if !ok || object == nil {
		return dto.Document{}, &dto.Problem{Code: "preferences.invalid", Status: 400}
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return dto.Document{}, &dto.Problem{Code: "preferences.invalid", Status: 400}
	}
	if len(encoded) > dto.SectionLimit {
		return dto.Document{}, &dto.Problem{Code: "preferences.too-large", Status: 413}
	}
	var document dto.Document
	err = s.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		var row struct{ UIPreferences string }
		result := tx.Table("users").Select("ui_preferences").Where("id=?", id).Take(&row)
		if errors.Is(result.Error, gorm.ErrRecordNotFound) {
			return &dto.Problem{Code: "preferences.no-user", Status: 403}
		}
		if result.Error != nil {
			return result.Error
		}
		document = decodeUIPreferences(ctx, row.UIPreferences)
		document.Sections[section] = object
		document.Revision++
		raw, err := json.Marshal(document)
		if err != nil {
			return err
		}
		if len(raw) > dto.DocumentLimit {
			return &dto.Problem{Code: "preferences.too-large", Status: 413}
		}
		return tx.Table("users").Where("id=?", id).Update("ui_preferences", string(raw)).Error
	})
	return document, err
}
