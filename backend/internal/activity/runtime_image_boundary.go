// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Hub validation of remote lifecycle evidence for #229.

package activity

import (
	"encoding/json"
	"maps"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

func checkedLifecycleResult(result dto.LifecycleResult) dto.LifecycleResult {
	valid := imageDTO.ValidateReason(result.Code) == nil && imageDTO.ValidateReason(result.BaselineCode) == nil
	if result.Code != "" {
		valid = valid && result.HTTPStatus >= 400 && result.HTTPStatus <= 599 && result.HTTPStatus != 401 && result.HTTPStatus != 403
	} else {
		valid = valid && (result.HTTPStatus == 0 || result.HTTPStatus == 200)
	}
	if !valid {
		return dto.LifecycleResult{RunID: result.RunID, Code: "runtime.outcome-unknown", HTTPStatus: 503}
	}
	return result
}

// Normalize before persistence so interactive waits, policy reads and exported
// metadata all consume the same declared evidence, including after restart.
func checkedRemoteLifecycle(outcome activitytypes.RuntimeExecutionOutcome) activitytypes.RuntimeExecutionOutcome {
	if outcome.Detail == nil {
		return outcome
	}
	detail := *outcome.Detail
	outcome.Detail = &detail
	checkedActivityLifecycle(&detail.Activity.Activity)
	return outcome
}

func checkedActivityLifecycle(item *activitytypes.Activity) {
	raw, exists := item.Metadata["imageLifecycle"]
	if !exists && item.ImageLifecycle == nil {
		return
	}
	result := dto.LifecycleResult{RunID: item.ID, Code: "runtime.outcome-unknown", HTTPStatus: 503}
	if exists {
		body, err := json.Marshal(raw)
		var decoded dto.LifecycleResult
		if err == nil && json.Unmarshal(body, &decoded) == nil && decoded.RunID == item.ID {
			result = checkedLifecycleResult(decoded)
		}
	} else if item.ImageLifecycle.RunID == item.ID {
		result = checkedLifecycleResult(*item.ImageLifecycle)
	}
	item.Metadata = maps.Clone(item.Metadata)
	if item.Metadata == nil {
		item.Metadata = map[string]any{}
	}
	item.Metadata["imageLifecycle"] = result
	item.ImageLifecycle = &result
}
