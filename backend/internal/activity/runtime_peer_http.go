// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

package activity

import (
	"github.com/danielgtaylor/huma/v2"
	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"strconv"
)

func runtimePeerHubCapable(ctx huma.Context) bool {
	version, err := strconv.Atoi(ctx.Header(activitytypes.RuntimeHubContractHeader))
	return err == nil && strconv.Itoa(version) == ctx.Header(activitytypes.RuntimeHubContractHeader) && version >= activitytypes.RuntimePeerContractVersion
}

// Missing/invalid announcement selects the frozen three-field refusal, with
// only legacy reason values. An old strict decoder never sees new members.
func runtimePeerLegacyProblem(problem *imageDTO.OperationError) *imageDTO.OperationError {
	copy := *problem
	copy.Problem = problem.Problem
	if b := problem.Problem.Blocker; b != nil {
		reason := b.Reason
		if reason != "compose.selection-unsupported" {
			reason = "compose.identity-unreadable"
		}
		copy.Problem.Blocker = &imageDTO.RuntimeIdentityBlocker{Name: b.Name, Reason: reason, Archived: b.Archived}
	}
	return &copy
}
