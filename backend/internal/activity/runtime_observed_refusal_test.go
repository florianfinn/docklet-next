// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original proofs for observed zero-effect refusals and the evidence capacity.

package activity

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
	"time"

	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	p "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type refusingProject struct{ observedAdapterProject }

func (*refusingProject) ResolveRuntimeProjectIdentity(context.Context, string, string) (p.Reference, bool, error) {
	return p.Reference{}, false, errors.New("docker unavailable")
}

// R1S1-2: an observed run refused before any effect records checked "not needed"
// evidence that the actual worker result mapper and terminal grammar keep known;
// plain runs record nothing, as before.
func TestObservedAdapterRefusalRecordsNotNeeded(t *testing.T) {
	for _, mode := range []string{"plain", "manual", "scheduled"} {
		t.Run(mode, func(t *testing.T) {
			adapter := NewRuntimeServiceAdapter(&refusingProject{}, &observedAdapterContainer{})
			var recorded *dto.LifecycleResult
			ctx := dto.WithResultSink(t.Context(), func(result dto.LifecycleResult) { recorded = &result })
			err := adapter.ExecuteRuntime(ctx, observedRefusalCommand(mode), io.Discard)
			var target *RuntimeTargetError
			require.True(t, errors.As(err, &target))
			require.Equal(t, "runtime.target-unavailable", target.Code)
			if mode == "plain" {
				require.Nil(t, recorded)
				return
			}
			require.NotNil(t, recorded)
			result := runtimeResultInternal(&runtimeOwnerInternal{ctx: ctx}, err, nil, false, time.Now())
			checked := checkedTerminalLifecycle("run", result.state, string(result.code), true, recorded)
			require.Equal(t, "runtime.target-unavailable", checked.Code)
			require.Equal(t, "not-needed", checked.Recovery.Outcome)
		})
	}
}

type deadlineProject struct{ observedAdapterProject }

func (*deadlineProject) ResolveRuntimeProjectIdentity(ctx context.Context, _, _ string) (p.Reference, bool, error) {
	<-ctx.Done()
	return p.Reference{}, false, ctx.Err()
}

// R2S1-2: the owner deadline ending during the read-only preflight makes the worker
// report runtime.interrupted; the zero-effect evidence stays known.
func TestObservedTimedOutRefusalStaysKnown(t *testing.T) {
	ctx, cancel := context.WithTimeout(t.Context(), 50*time.Millisecond)
	defer cancel()
	adapter := NewRuntimeServiceAdapter(&deadlineProject{}, &observedAdapterContainer{})
	var recorded *dto.LifecycleResult
	ctx = dto.WithResultSink(ctx, func(result dto.LifecycleResult) { recorded = &result })
	err := adapter.ExecuteRuntime(ctx, observedRefusalCommand("manual"), io.Discard)
	require.Error(t, err)
	require.NotNil(t, recorded)
	result := runtimeResultInternal(&runtimeOwnerInternal{ctx: ctx}, err, nil, false, time.Now())
	require.Equal(t, "runtime.interrupted", string(result.code))
	checked := checkedTerminalLifecycle("run", result.state, string(result.code), true, recorded)
	require.NotEqual(t, "runtime.outcome-unknown", checked.Code)
	require.Equal(t, "not-needed", checked.Recovery.Outcome)
}

func observedRefusalCommand(mode string) RuntimeCommand {
	intent := &dto.LifecycleIntent{Request: dto.LifecycleRequest{AutoRestore: mode == "manual"}}
	if mode == "scheduled" {
		intent.Policy = &dto.PolicyBinding{PolicyID: "policy", ChildID: "child"}
	}
	return RuntimeCommand{Operation: RuntimeUpdateProject, ProposedRunID: "run", Target: RuntimeTarget{Kind: "stack", ID: "stack", Name: new("stack")}, Options: RuntimeOptions{Lifecycle: intent}}
}

// R2S1-1: evidence at the cap stays readable by the executor detail producer, with
// history present and after its deletion.
func TestObservedEvidenceAtCapThroughDetail(t *testing.T) {
	f := runtimeFixture(t)
	req := f.request()
	req.OperationID = string(RuntimeUpdateProject)
	row, err := f.admit(req)
	require.NoError(t, err)
	require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: "scheduled", CreatedAt: f.now}).Error)
	result := dto.LifecycleResult{RunID: row.RunID, Code: "update.unstable", HTTPStatus: 500, Recovery: &dto.RecoveryResult{Outcome: "restored"}}
	body, err := json.Marshal(result)
	require.NoError(t, err)
	for len(body) < dto.MaxRecoveryEvidenceBytes-1200 {
		result.Applied = append(result.Applied, dto.AppliedCandidate{Service: strings.Repeat("s", 128), Repository: strings.Repeat("r", 600), Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("a", 64)})
		body, err = json.Marshal(result)
		require.NoError(t, err)
	}
	padding := dto.MaxRecoveryEvidenceBytes - len(body)
	for i := range result.Applied {
		add := min(1024-len(result.Applied[i].Repository), padding)
		result.Applied[i].Repository += strings.Repeat("x", add)
		if padding -= add; padding == 0 {
			break
		}
	}
	body, err = json.Marshal(result)
	require.NoError(t, err)
	require.Len(t, body, dto.MaxRecoveryEvidenceBytes)
	require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
		snapshot, _, err := runtimeCompleteTxInternal(tx.WithContext(context.WithValue(tx.Statement.Context, gateRunContextKey{}, row.RunID)), now, req.Identity, runtimeLocalResultInternal{state: "failed", code: activityDTO.CodeRuntimeFailed, message: "failed", settledAt: now, imageLifecycle: &result})
		require.NoError(t, err)
		require.Equal(t, activityDTO.RuntimeOutcomeKnown, snapshot.OutcomeState)
		return err
	}))
	require.NoError(t, f.svc.db.First(row, "run_id = ?", row.RunID).Error)
	detail, err := runtimeDetailTxInternal(f.svc.db.DB, row)
	require.NoError(t, err)
	require.Equal(t, &result, detail.Activity.ImageLifecycle)
	_, err = f.svc.DeleteHistory(t.Context(), "0")
	require.NoError(t, err)
	detail, err = runtimeDetailTxInternal(f.svc.db.DB, row)
	require.NoError(t, err)
	require.Equal(t, &result, detail.Activity.ImageLifecycle)
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

// R3S1-1: a "not needed" recovery code that contradicts the lifecycle code is unknown
// for every terminal status, through the real completion transaction.
func TestObservedNotNeededCodeContradictionIsUnknown(t *testing.T) {
	for _, mode := range []string{"manual", "scheduled"} {
		for _, terminal := range []struct {
			name, state string
			code        activityDTO.Code
		}{
			{"ordinary", "failed", activityDTO.CodeRuntimeFailed},
			{"interrupted", "failed", activityDTO.CodeRuntimeInterrupted},
			{"cancelled", "cancelled", activityDTO.CodeRuntimeCancelled},
		} {
			for _, reason := range []string{"", "runtime.target-unavailable", "update.deploy-failed"} {
				t.Run(mode+"/"+terminal.name+"/"+reason, func(t *testing.T) {
					f := runtimeFixture(t)
					req := f.request()
					req.OperationID = string(RuntimeUpdateProject)
					row, err := f.admit(req)
					require.NoError(t, err)
					require.NoError(t, f.svc.db.Create(&RuntimeObservedRun{RunID: row.RunID, Kind: "stack", Mode: mode, CreatedAt: f.now}).Error)
					result := dto.LifecycleResult{RunID: row.RunID, Code: "runtime.target-unavailable", HTTPStatus: 503, Recovery: &dto.RecoveryResult{Outcome: "not-needed", Code: reason}}
					want := activityDTO.RuntimeOutcomeKnown
					if reason == "update.deploy-failed" {
						want = activityDTO.RuntimeOutcomeUnknown
					}
					require.NoError(t, f.tx(func(tx *gorm.DB, now time.Time) error {
						snapshot, _, err := runtimeCompleteTxInternal(tx.WithContext(context.WithValue(tx.Statement.Context, gateRunContextKey{}, row.RunID)), now, req.Identity, runtimeLocalResultInternal{state: terminal.state, code: terminal.code, message: "refused", settledAt: now, imageLifecycle: &result})
						require.NoError(t, err)
						require.Equal(t, want, snapshot.OutcomeState)
						return err
					}))
				})
			}
		}
	}
}
