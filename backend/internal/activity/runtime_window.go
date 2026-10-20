// Source: docklet-next/docs/design/runtime-control.md
// Revision: 230a4a3cb92c59cc15e1f6767abd18f017e05ca4b83835524aa8c20e50a89e46
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Dormant bounded remote runtime implementation; production activation is separate.

package activity

import (
	"database/sql/driver"
	"encoding/json/v2"
	"errors"
	"math"
	"unicode/utf8"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"gorm.io/gorm"
)

// RuntimeOutputLines is a bounded window, independent of the compact next cursor.
// Clearing Activity history never resets the receipt's sequence allocator.
type RuntimeOutputLines []activitytypes.RuntimeExecutionOutputLine

func (v RuntimeOutputLines) Value() (driver.Value, error) {
	if v == nil {
		v = RuntimeOutputLines{}
	}
	return database.JSONValue(false, []activitytypes.RuntimeExecutionOutputLine(v))
}
func (v *RuntimeOutputLines) Scan(value any) error { return database.JSONScan(v, value) }

func runtimeAppendOutputTxInternal(tx *gorm.DB, current *Activity, updates map[string]any, incoming []activitytypes.OutputLine, retain bool) error {
	if len(incoming) == 0 {
		return nil
	}
	var row RuntimeReceipt
	err := tx.First(&row, "run_id = ? AND kind = 'receipt'", current.ID).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		if retain {
			lines, cut := appendOutputLinesInternal(current.Output, incoming)
			updates["output"], updates["output_truncated"] = lines, current.OutputTruncated || cut
		}
		return nil
	}
	if err != nil {
		return err
	}
	if row.OutputNextSeq == 0 || row.OutputNextSeq > math.MaxInt64-uint64(len(incoming)) {
		return errRuntimeInvalidInternal
	}
	next := row.OutputNextSeq
	window := append(RuntimeOutputLines{}, current.RuntimeOutput...)
	cut := current.OutputTruncated || updates["output_truncated"] == true
	for _, line := range incoming {
		entry := activitytypes.RuntimeExecutionOutputLine{OutputLine: line, OutputSeq: next}
		next++
		if !retain {
			cut = true
			continue
		}
		// Reuse the established UTF-8 tail cut, then account for the sequence field.
		single, truncated := appendOutputLinesInternal(nil, []activitytypes.OutputLine{line})
		entry.OutputLine = single[0]
		cut = cut || truncated
		encoded, err := json.Marshal(entry)
		if err != nil {
			return err
		}
		for len(encoded)+2 > maxSerializedOutputBytes {
			if entry.Text == "" {
				return errRuntimeInvalidInternal
			}
			_, n := utf8.DecodeRuneInString(entry.Text)
			entry.Text = entry.Text[n:]
			cut = true
			encoded, err = json.Marshal(entry)
			if err != nil {
				return err
			}
		}
		window = append(window, entry)
	}
	size := 2
	sizes := make([]int, len(window))
	for i, line := range window {
		b, err := json.Marshal(line)
		if err != nil {
			return err
		}
		sizes[i] = len(b)
		size += len(b)
		if i > 0 {
			size++
		}
	}
	for len(window) > 0 && size > maxSerializedOutputBytes {
		size -= sizes[0]
		if len(window) > 1 {
			size--
		}
		sizes = sizes[1:]
		window = window[1:]
		cut = true
	}
	changed := tx.Model(&RuntimeReceipt{}).Where("run_id = ? AND output_next_seq = ?", row.RunID, row.OutputNextSeq).UpdateColumn("output_next_seq", next)
	if changed.Error != nil {
		return changed.Error
	}
	if changed.RowsAffected != 1 {
		return &runtimeConflictInternal{}
	}
	legacy := make(OutputLines, 0, len(window))
	for _, line := range window {
		legacy = append(legacy, line.OutputLine)
	}
	updates["runtime_output"], updates["output"], updates["output_truncated"] = window, legacy, cut
	return nil
}

// RuntimeStepTrace retains only the current bounded executor projection window.
type RuntimeStepTrace []activitytypes.StepTraceEntry

func (v RuntimeStepTrace) Value() (driver.Value, error) {
	if v == nil {
		v = RuntimeStepTrace{}
	}
	return database.JSONValue(false, []activitytypes.StepTraceEntry(v))
}
func (v *RuntimeStepTrace) Scan(value any) error { return database.JSONScan(v, value) }
