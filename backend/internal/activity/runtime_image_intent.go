// Source: docklet-next/docs/design/images-updates.md
// Revision: 5a7a7ad39c1817a3d222bc84c0cf91d433d48f0d
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Frozen image intent retention for receipt-first retries; no effect replay.

package activity

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/types/updater"
	"gorm.io/gorm"
)

type RuntimeImageIntent struct {
	RunID   string `gorm:"primaryKey;not null"`
	Payload string `gorm:"not null"`
}

func (RuntimeImageIntent) TableName() string { return "runtime_image_intents" }
func runtimeImageOperation(op RuntimeOperation) bool {
	return op == RuntimeUpdateProject || op == RuntimeRestoreProject
}
func runtimeSaveImageIntent(tx *gorm.DB, run string, c RuntimeCommand) error {
	if !runtimeImageOperation(c.Operation) {
		return nil
	}
	body, err := json.Marshal(c.Options.Lifecycle)
	if err != nil {
		return err
	}
	return tx.Create(&RuntimeImageIntent{RunID: run, Payload: string(body)}).Error
}
func (s *RuntimeService) retainedImageIntent(ctx context.Context, run string, c RuntimeCommand) (RuntimeCommand, error) {
	if !runtimeImageOperation(c.Operation) {
		return c, nil
	}
	var row RuntimeImageIntent
	if err := s.activity.db.WithContext(ctx).First(&row, "run_id = ?", run).Error; err != nil {
		return c, &RuntimeStoreError{Cause: err}
	}
	var intent updater.LifecycleIntent
	if json.Unmarshal([]byte(row.Payload), &intent) != nil || c.Options.Lifecycle == nil || c.Options.Lifecycle.Request != intent.Request {
		return c, &runtimeConflictInternal{}
	}
	c.Options = RuntimeOptions{Lifecycle: &intent}
	return c, nil
}

func runtimeImageResult(model *Activity) *updater.LifecycleResult {
	if model == nil || model.Metadata == nil {
		return nil
	}
	raw, ok := model.Metadata["imageLifecycle"]
	if !ok {
		return nil
	}
	body, err := json.Marshal(raw)
	if err != nil {
		return nil
	}
	var result updater.LifecycleResult
	if json.Unmarshal(body, &result) != nil || result.RunID != model.ID {
		return nil
	}
	return &result
}

func runtimeImageResultTx(tx *gorm.DB, run string, result *updater.LifecycleResult) error {
	if result == nil || result.RunID != run {
		return errRuntimeInvalidInternal
	}
	var model Activity
	if err := tx.First(&model, "id = ?", run).Error; err != nil {
		return err
	}
	if model.Metadata == nil {
		model.Metadata = database.JSON{}
	}
	body, err := json.Marshal(result)
	if err != nil {
		return err
	}
	var safe map[string]any
	if err = json.Unmarshal(body, &safe); err != nil {
		return err
	}
	model.Metadata["imageLifecycle"] = safe
	return tx.Model(&Activity{}).Where("id = ?", run).UpdateColumn("metadata", model.Metadata).Error
}
