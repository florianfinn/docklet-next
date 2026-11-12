// Source: docklet-next/issues/440
// Revision: 45ee2635d68e71671b571cbd8970425ddd194b59
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proofs for joined observed outcomes against later cancellation.

package activity

import (
	"context"
	"testing"
	"time"

	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
)

// Q-CAN: a joined recovery outcome after the first effect stands; a cancel that
// arrived meanwhile neither turns it into cancelled nor into an interruption.
func TestObservedJoinedRecoveryWinsOverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failure := imageDTO.Failure("update.unstable", 500)
	for _, tc := range []struct {
		outcome string
		state   string
		code    activityDTO.Code
	}{
		{"restored", "failed", activityDTO.CodeRuntimeFailed},
		{"restore-failed", "failed", activityDTO.CodeRuntimeFailed},
		{"unavailable", "failed", activityDTO.CodeRuntimeFailed},
		{"not-needed", "cancelled", activityDTO.CodeRuntimeCancelled},
	} {
		t.Run(tc.outcome, func(t *testing.T) {
			lifecycle := &dto.LifecycleResult{Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: tc.outcome}}
			owner := &runtimeOwnerInternal{ctx: ctx, cancelRequested: true}
			result := runtimeResultInternal(owner, failure, nil, runtimeJoinedRecovery(lifecycle), time.Now())
			require.Equal(t, tc.state, result.state)
			require.Equal(t, tc.code, result.code)
		})
	}
}

// #409.2: a refused success that recorded its replacement is joined as well,
// so the failed+R shape is kept instead of a cancelled run carrying R.
func TestRefusedReplacementWinsOverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	owner := &runtimeOwnerInternal{ctx: ctx, cancelRequested: true}
	result := runtimeResultInternal(owner, imageDTO.Failure("update.target-changed", 409), nil, true, time.Now())
	require.Equal(t, "failed", result.state)
	require.Equal(t, activityDTO.CodeRuntimeFailed, result.code)
}

type joinedTestFailure struct{ error }

func (e joinedTestFailure) Unwrap() error       { return e.error }
func (e joinedTestFailure) RuntimeJoined() bool { return true }

// R1S3-4: a detached failure after the first workload effect carries no R and
// Recovery not-needed; its joined marker keeps it against cancel and deadline.
func TestObservedJoinedFailureWinsOverCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	failure := joinedTestFailure{imageDTO.Failure("update.deploy-failed", 500)}
	lifecycle := &dto.LifecycleResult{Code: "update.deploy-failed", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "not-needed"}}
	for _, cancelled := range []bool{true, false} {
		owner := &runtimeOwnerInternal{ctx: ctx, cancelRequested: cancelled}
		result := runtimeResultInternal(owner, failure, nil, runtimeJoinedRecovery(lifecycle), time.Now())
		require.Equal(t, "failed", result.state)
		require.Equal(t, activityDTO.CodeRuntimeFailed, result.code)
	}
}
