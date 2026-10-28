// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original no-replay settlement for superseded volume backup actors.

package backup

import (
	"context"
	"encoding/json"
	"github.com/florianfinn/docklet-next/types/scheduler"
)

// SettleVolumeRuns retires old actor intents before coordinator activation.
// Volume captures have SQL-owned recovery and must never replay Docker work.
func (e *Engine) SettleVolumeRuns(ctx context.Context, fenced func(string) bool) error {
	if e == nil || e.service == nil {
		return nil
	}
	states, err := e.activeRunsInternal(ctx)
	if err != nil {
		return err
	}
	for _, state := range states {
		if state.Command.Kind != "volume" {
			continue
		}
		var payload struct {
			VolumeName string `json:"volumeName"`
		}
		if err = json.Unmarshal(state.Command.Payload, &payload); err != nil {
			return err
		}
		if fenced != nil && fenced(payload.VolumeName) {
			continue
		}
		state.Status = scheduler.Failed
		state.Error = "volume.backup.interrupted"
		if err = e.service.SetState(ctx, backupRunTypeInternal, state.Command.RunID, state, nil); err != nil {
			return err
		}
	}
	return nil
}
