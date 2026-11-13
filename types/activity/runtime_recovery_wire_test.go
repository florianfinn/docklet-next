// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original narrow failed replacement result wire proofs. M24 recovery evidence, validation and reason contracts.

package activity

import (
	c "github.com/florianfinn/docklet-next/types/container"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"strings"
	"testing"
	"time"
)

func TestRecoveryFailedReplacementWireShapes(t *testing.T) {
	for _, shape := range []string{"target-changed", "interrupted"} {
		for _, mutation := range []string{"valid", "status", "operation", "type", "runtime-code", "lifecycle-code", "old-id", "new-id", "names", "verified"} {
			t.Run(shape+"/"+mutation, func(t *testing.T) {
				id := runtimeWireIdentityFixture()
				now := time.Now()
				a := RuntimeSnapshot{Operation: RuntimeUpdateContainer, Activity: Activity{ID: id.RunID, EnvironmentID: "0", Type: TypeContainerRedeploy, Status: StatusFailed, StartedAt: now, CreatedAt: now, EndedAt: &now, ResourceID: new(strings.Repeat("a", 64)), MessageCode: "runtime.failed", ImageLifecycle: &dto.LifecycleResult{Code: "update.target-changed"}}, RuntimeKnowledge: RuntimeKnowledge{OutcomeState: RuntimeOutcomeKnown}, Result: &c.RecreateResult{OldID: strings.Repeat("a", 64), NewID: strings.Repeat("b", 64), OldName: "app", NewName: "app"}}
				if shape == "interrupted" {
					a.MessageCode = "runtime.interrupted"
				}
				wantErr := mutation != "valid"
				switch mutation {
				case "status":
					a.Status = StatusCancelled
				case "operation":
					a.Operation = RuntimeRestoreContainerImage
				case "type":
					a.Type = TypeContainerEdit
				case "runtime-code":
					a.MessageCode = "activity.failed"
				case "lifecycle-code":
					a.ImageLifecycle.Code = "update.unstable"
					if shape == "interrupted" {
						wantErr = false
					}
				case "old-id":
					a.Result.OldID = strings.Repeat("c", 64)
				case "new-id":
					a.Result.NewID = a.Result.OldID
				case "verified":
					a.ImageLifecycle.Verified = true
				case "names":
					a.Result.NewName = ""
				}
				o := RuntimeExecutionOutcome{Identity: id, State: RuntimeAnswerTerminal, Detail: &RuntimeExecutionDetail{Activity: a, Messages: []Message{}, StepTrace: []StepTraceEntry{}, Output: []RuntimeExecutionOutputLine{}, RetainedStartSeq: 1}}
				require.Equal(t, wantErr, ValidateRuntimeOutcome(o, id) != nil)
			})
		}
	}
}
