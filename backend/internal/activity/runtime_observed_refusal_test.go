// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proofs for observed zero-effect refusals and the evidence capacity.

package activity

import (
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	p "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

type refusingProject struct{ observedAdapterProject }

func (*refusingProject) ResolveRuntimeProjectIdentity(context.Context, string, string) (p.Reference, bool, error) {
	return p.Reference{}, false, errors.New("docker unavailable")
}

// R1S1-2: an observed run refused before any effect records checked "not needed"
// evidence, so it settles failed instead of becoming a held unknown; plain runs
// record nothing, as before.
func TestObservedAdapterRefusalRecordsNotNeeded(t *testing.T) {
	for _, mode := range []string{"plain", "manual", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			adapter := NewRuntimeServiceAdapter(&refusingProject{}, &observedAdapterContainer{})
			intent := &dto.LifecycleIntent{Request: dto.LifecycleRequest{AutoRestore: mode == "manual"}}
			if mode == "scheduled" {
				intent.Policy = &dto.PolicyBinding{PolicyID: "policy", ChildID: "child"}
			}
			command := RuntimeCommand{Operation: RuntimeUpdateProject, ProposedRunID: "run", Target: RuntimeTarget{Kind: "stack", ID: "stack", Name: new("stack")}, Options: RuntimeOptions{Lifecycle: intent}}
			var recorded *dto.LifecycleResult
			ctx := dto.WithResultSink(t.Context(), func(result dto.LifecycleResult) { recorded = &result })
			var target *RuntimeTargetError
			require.True(t, errors.As(adapter.ExecuteRuntime(ctx, command, io.Discard), &target))
			require.Equal(t, "runtime.target-unavailable", target.Code)
			if mode == "plain" {
				require.Nil(t, recorded)
				return
			}
			require.NotNil(t, recorded)
			checked := checkedTerminalLifecycle("run", "failed", "runtime.failed", true, recorded)
			require.Equal(t, "runtime.target-unavailable", checked.Code)
			require.Equal(t, "not-needed", checked.Recovery.Outcome)
		})
	}
}

// R1S1-1: the evidence of a stack with the maximum number of applied services and
// realistic names stays known.
func TestObservedEvidenceHoldsMaximumAppliedServices(t *testing.T) {
	result := dto.LifecycleResult{RunID: "run", Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}
	for i := range dto.MaxAppliedCandidates {
		result.Applied = append(result.Applied, dto.AppliedCandidate{Service: "service-name-" + strings.Repeat("x", 4) + string(rune('a'+i%26)), Repository: "ghcr.io/example-org/service-name", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)})
	}
	checked := checkedTerminalLifecycle("run", "failed", "runtime.failed", true, &result)
	require.Equal(t, "update.unstable", checked.Code)
	require.Len(t, checked.Applied, dto.MaxAppliedCandidates)
}
