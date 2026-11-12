// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Hub validation of remote lifecycle evidence for #229; M24 checked recovery grammar, bounded evidence and contradictory-copy rejection.

package activity

import (
	"encoding/json"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"maps"
	"reflect"
	"slices"
)

func unknownLifecycle(run string) dto.LifecycleResult {
	return dto.LifecycleResult{RunID: run, Code: "runtime.outcome-unknown", HTTPStatus: 503}
}

// Structural ingress validation precedes trusted receipt/marker semantics.
func checkedLifecycleResult(result dto.LifecycleResult) dto.LifecycleResult {
	valid := imageDTO.ValidateReason(result.Code) == nil && imageDTO.ValidateReason(result.BaselineCode) == nil && imageDTO.ValidateReason(result.PointCode) == nil
	if result.Code != "" {
		valid = valid && result.HTTPStatus >= 400 && result.HTTPStatus <= 599 && result.HTTPStatus != 401 && result.HTTPStatus != 403
	} else {
		valid = valid && (result.HTTPStatus == 0 || result.HTTPStatus == 200)
	}
	valid = valid && len(result.RestorePointID) <= 36 && (result.RestorePointID == "" || runtimeTextInternal(result.RestorePointID, 36)) && len(result.Applied) <= dto.MaxAppliedCandidates
	for _, value := range result.Applied {
		valid = valid && runtimeSuppressedValid([]dto.SuppressedCandidate{value})
	}
	if r := result.Recovery; r != nil {
		valid = valid && slices.Contains([]string{"restored", "restore-failed", "unavailable", "not-needed", "interrupted"}, r.Outcome) && imageDTO.ValidateReason(r.Code) == nil
	}
	body, err := json.Marshal(result)
	if !valid || err != nil || len(body) > dto.MaxRecoveryEvidenceBytes {
		return unknownLifecycle(result.RunID)
	}
	result.Applied = slices.Clone(result.Applied)
	if len(result.Applied) == 0 {
		result.Applied = nil
	}
	if result.Recovery != nil {
		copy := *result.Recovery
		result.Recovery = &copy
	}
	return result
}

// terminalShape is what checked evidence may claim for one terminal status and code.
type terminalShape struct {
	evidenceOptional, recoveryOptional, verified, codeRequired bool
	outcomes                                                   []string
}

func observedTerminalShape(status, code string) (terminalShape, bool) {
	switch {
	case status == "success":
		return terminalShape{recoveryOptional: true, verified: true, outcomes: []string{"not-needed"}}, true
	case status == "cancelled", status == "failed" && code == "runtime.interrupted":
		// Interruption and cancellation may end a run before or after its first effect.
		return terminalShape{evidenceOptional: true, recoveryOptional: true, outcomes: []string{"interrupted", "not-needed"}}, true
	case status == "failed":
		return terminalShape{codeRequired: true, outcomes: []string{"not-needed", "restored", "restore-failed", "unavailable"}}, true
	}
	return terminalShape{}, false
}

// recoveryCodeValid is the per-outcome reason rule; it never depends on the terminal status.
func recoveryCodeValid(result dto.LifecycleResult) bool {
	recovery := result.Recovery
	switch recovery.Outcome {
	case "not-needed", "interrupted":
		return recovery.Code == "" || recovery.Code == result.Code
	case "restored":
		return recovery.Code == ""
	case "restore-failed":
		return true
	case "unavailable":
		return slices.Contains([]string{"restore.unavailable", "restore.files-changed", "restore.shared-reference", "image.busy", "container.auto-remove"}, recovery.Code)
	}
	return false
}

func checkedTerminalLifecycle(run, status, code string, bound bool, result *dto.LifecycleResult) *dto.LifecycleResult {
	unknown := func() *dto.LifecycleResult { value := unknownLifecycle(run); return &value }
	if !bound {
		if result == nil {
			return nil
		}
		if checked := checkedLifecycleResult(*result); checked.RunID == run && checked.Code != "runtime.outcome-unknown" && checked.Recovery == nil {
			return &checked
		}
		return unknown()
	}
	shape, ok := observedTerminalShape(status, code)
	if !ok {
		return unknown()
	}
	if result == nil {
		if shape.evidenceOptional {
			return nil
		}
		return unknown()
	}
	checked := checkedLifecycleResult(*result)
	if checked.RunID != run || checked.Code == "runtime.outcome-unknown" || checked.Verified != shape.verified ||
		shape.verified && checked.Code != "" || shape.codeRequired && checked.Code == "" {
		return unknown()
	}
	if checked.Recovery == nil {
		if shape.recoveryOptional {
			return &checked
		}
		return unknown()
	}
	if !slices.Contains(shape.outcomes, checked.Recovery.Outcome) || !recoveryCodeValid(checked) {
		return unknown()
	}
	return &checked
}

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
	result := unknownLifecycle(item.ID)
	if exists {
		body, err := json.Marshal(raw)
		var decoded dto.LifecycleResult
		if err == nil && json.Unmarshal(body, &decoded) == nil && decoded.RunID == item.ID {
			result = checkedLifecycleResult(decoded)
			if item.ImageLifecycle != nil && !reflect.DeepEqual(checkedLifecycleResult(*item.ImageLifecycle), result) {
				result = unknownLifecycle(item.ID)
			}
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
