// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed authority and startup result/code dispatch proofs.

package activity

import (
	"context"
	c "github.com/florianfinn/docklet-next/types/container"
	p "github.com/florianfinn/docklet-next/types/project"
	dto "github.com/florianfinn/docklet-next/types/updater"
	u "github.com/florianfinn/docklet-next/types/user"
	"github.com/stretchr/testify/require"
	"io"
	"strings"
	"testing"
)

type observedAdapterProject struct {
	runtimeProjectPort
	runtimeImageLifecyclePort
	seen dto.ObservedExecution
}

func (p *observedAdapterProject) ResolveRuntimeProjectIdentity(context.Context, string, string) (p.Reference, bool, error) {
	return structProjectReference(), true, nil
}
func structProjectReference() p.Reference { return p.Reference{ID: "stack", Name: "stack"} }
func (p *observedAdapterProject) ExecuteImageLifecycle(ctx context.Context, _ string, _ string, _ string, _ *dto.LifecycleIntent, _ u.Actor) (*dto.LifecycleResult, error) {
	p.seen = dto.ObservedExecutionFromContext(ctx)
	return nil, nil
}

type observedAdapterContainer struct {
	runtimeContainerPort
	runtimeContainerRecreatePort
	seen      dto.ObservedExecution
	recovered c.RecreateRecovery
}

func (p *observedAdapterContainer) ExecuteContainerImage(ctx context.Context, _ string, _ string, _ bool, _ u.Actor) error {
	p.seen = dto.ObservedExecutionFromContext(ctx)
	return nil
}
func (p *observedAdapterContainer) RecoverRecreates(context.Context) ([]c.RecreateRecovery, []string, error) {
	return []c.RecreateRecovery{p.recovered}, nil, nil
}
func (p *observedAdapterContainer) PruneRecreateJournals(context.Context) error { return nil }
func TestObservedAdapterAuthority(t *testing.T) {
	for _, op := range []RuntimeOperation{RuntimeUpdateProject, RuntimeRestoreProject, RuntimeUpdateContainer, RuntimeRestoreContainerImage} {
		for _, mode := range []string{"plain", "manual", "scheduled"} {
			t.Run(string(op)+"/"+mode, func(t *testing.T) {
				project := &observedAdapterProject{}
				container := &observedAdapterContainer{}
				adapter := NewRuntimeServiceAdapter(project, container)
				command := RuntimeCommand{Operation: op, Target: RuntimeTarget{Kind: "stack", ID: "stack", Name: new("stack")}, Options: RuntimeOptions{Lifecycle: &dto.LifecycleIntent{}}}
				var binding *dto.PolicyBinding
				if mode == "scheduled" {
					binding = &dto.PolicyBinding{PolicyID: "policy", ChildID: "child", Suppressed: []dto.SuppressedCandidate{{Repository: "registry.example.com/app", Platform: "linux/amd64", Manifest: "sha256:" + strings.Repeat("f", 64)}}}
				}
				command.Options.Lifecycle.Policy = binding
				command.Options.Lifecycle.Request.AutoRestore = mode == "manual"
				if op == RuntimeUpdateContainer || op == RuntimeRestoreContainerImage {
					command.Target = RuntimeTarget{Kind: "container", ID: strings.Repeat("a", 64), Name: new("app")}
					command.Options = RuntimeOptions{ContainerImage: &dto.ContainerImageRequest{AutoRestore: mode == "manual", Policy: binding}}
				}
				require.NoError(t, adapter.ExecuteRuntime(t.Context(), command, io.Discard))
				seen := project.seen
				if command.Target.Kind == "container" {
					seen = container.seen
				}
				expected := dto.ObservedExecution{}
				if op == RuntimeUpdateContainer || op == RuntimeUpdateProject {
					if mode == "manual" {
						expected.Mode = "manual"
					}
					if mode == "scheduled" {
						expected = dto.ObservedExecution{Mode: "scheduled", PolicyID: "policy", ChildID: "child", Suppressed: binding.Suppressed}
					}
				}
				require.Equal(t, expected, seen)
			})
		}
	}
}
func TestObservedStartupResultAndCodeStaysFailed(t *testing.T) {
	f := runtimeFixture(t)
	request := f.request()
	request.OperationID = string(RuntimeUpdateContainer)
	request.TargetKind = "container"
	request.TargetID = strings.Repeat("a", 64)
	request.TargetName = new("app")
	request.GateScope = "container:" + request.TargetID
	row, err := f.admit(request)
	require.NoError(t, err)
	result := &c.RecreateResult{OldID: request.TargetID, NewID: strings.Repeat("b", 64), OldName: "app", NewName: "app"}
	port := &observedAdapterContainer{recovered: c.RecreateRecovery{RunID: row.RunID, Result: result, Code: "update.target-changed"}}
	adapter := NewRuntimeServiceAdapter(nil, port)
	service := NewRuntimeService(f.svc, adapter)
	store := RuntimeStoreBinding{StoreGeneration: f.generation, ExecutorGeneration: f.generation, ManagerID: request.Identity.ManagerID, RegistrationID: request.Identity.RegistrationID, EnvironmentID: "0"}
	require.NoError(t, service.ConfigureLocal(RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: adapter}))
	require.NoError(t, service.recoverRecreatesStartup(t.Context()))
	snapshot, err := runtimeSnapshotTxInternal(f.svc.db.DB, row)
	require.NoError(t, err)
	// Reload the receipt: the pre-recovery value has deliberately stale fields.
	var receipt RuntimeReceipt
	require.NoError(t, f.svc.db.First(&receipt, "run_id = ?", row.RunID).Error)
	snapshot, err = runtimeSnapshotTxInternal(f.svc.db.DB, &receipt)
	require.NoError(t, err)
	require.Equal(t, "failed", string(snapshot.Status))
	require.Equal(t, "runtime.failed", snapshot.MessageCode)
	require.Equal(t, "update.target-changed", snapshot.ImageLifecycle.Code)
	require.False(t, snapshot.ImageLifecycle.Verified)
	require.Equal(t, result, snapshot.Result)
}
