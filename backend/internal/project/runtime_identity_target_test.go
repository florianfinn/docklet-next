// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Counted target-only database, input and identity loads at admission and pre-effect dispatch for M35 #558.

package project

import (
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/pkg/projects"
	activityDTO "github.com/florianfinn/docklet-next/types/activity"
	usertypes "github.com/florianfinn/docklet-next/types/user"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

type runtimeTargetReads struct {
	mu     sync.Mutex
	inputs map[string]int
	models map[string]int
	rows   []string
}

func countRuntimeTargetReads(t *testing.T, svc *ProjectService) *runtimeTargetReads {
	t.Helper()
	reads := &runtimeTargetReads{inputs: map[string]int{}, models: map[string]int{}}
	inputs, load := runtimeProjectInputs, runtimeLoadIdentity
	runtimeProjectInputs = func(s *ProjectService, ctx context.Context, dir string) (context.Context, *projects.ResolvedInputs, error) {
		reads.mu.Lock()
		reads.inputs[dir]++
		reads.mu.Unlock()
		return inputs(s, ctx, dir)
	}
	runtimeLoadIdentity = func(ctx context.Context, file, name, root string, inject bool) (*compose.Project, []string, error) {
		reads.mu.Lock()
		reads.models[file]++
		reads.mu.Unlock()
		return load(ctx, file, name, root, inject)
	}
	const callback = "test:runtime-target-reads"
	require.NoError(t, svc.db.Callback().Query().After("gorm:query").Register(callback, func(db *gorm.DB) {
		if rows, ok := db.Statement.Dest.(*[]Project); ok {
			reads.mu.Lock()
			defer reads.mu.Unlock()
			for _, row := range *rows {
				reads.rows = append(reads.rows, row.ID)
			}
		}
	}))
	t.Cleanup(func() {
		runtimeProjectInputs, runtimeLoadIdentity = inputs, load
		require.NoError(t, svc.db.Callback().Query().Remove(callback))
	})
	return reads
}

func (r *runtimeTargetReads) assertAndReset(t *testing.T, phase string, target *Project, exists bool) {
	t.Helper()
	r.mu.Lock()
	defer r.mu.Unlock()
	inputs, models, rows := map[string]int{}, map[string]int{}, []string(nil)
	if exists {
		inputs[target.Path] = 1
		models[filepath.Join(target.Path, "compose.yaml")] = 2
		rows = []string{target.ID}
	}
	assert.Equal(t, inputs, r.inputs, "%s resolves only target inputs", phase)
	assert.Equal(t, models, r.models, "%s keeps both fresh target identity loads and reads no peer models", phase)
	assert.Equal(t, rows, r.rows, "%s loads only the target database row", phase)
	r.inputs, r.models, r.rows = map[string]int{}, map[string]int{}, nil
}

// The port records dispatch after the real adapter's pre-effect identity guard.
// Existing SDK and Direct/Edge suites cover the downstream project effects.
type runtimeTargetEffectPort struct {
	*ProjectService
	effects int
}

func (p *runtimeTargetEffectPort) DownProject(context.Context, string, usertypes.Actor) error {
	p.effects++
	return nil
}

type runtimeTargetHeldExecutor struct {
	adapter          *activity.RuntimeServiceAdapter
	entered, release chan struct{}
	err              error
}

func (e *runtimeTargetHeldExecutor) ExecuteRuntime(ctx context.Context, command activity.RuntimeCommand, output io.Writer) error {
	close(e.entered)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-e.release:
	}
	e.err = e.adapter.ExecuteRuntime(ctx, command, output)
	return e.err
}

func TestRuntimeIdentityTargetOnlyAdmissionAndPreEffectReads(t *testing.T) {
	for _, change := range []string{"unchanged", "renamed", "deleted"} {
		t.Run(change, func(t *testing.T) {
			svc, root := runtimeIdentityFixture(t)
			target := runtimeIdentityProject(t, svc, root, "example-target", "target", "name: target_stack\n"+identityServices)
			const peers = 18
			for i := range peers {
				body := "not: [valid"
				switch i % 3 {
				case 1:
					body = "name: huge_peer\n" + identityServices + "#" + strings.Repeat("x", 1<<20) + "\n"
				case 2:
					body = "name: target_stack\n" + identityServices
				}
				runtimeIdentityProject(t, svc, root, fmt.Sprintf("example-peer-%02d", i), fmt.Sprintf("peer-%02d", i), body)
			}
			reads := countRuntimeTargetReads(t, svc)
			port := &runtimeTargetEffectPort{ProjectService: svc}
			adapter := activity.NewRuntimeServiceAdapter(port, nil)
			held := &runtimeTargetHeldExecutor{adapter: adapter, entered: make(chan struct{}), release: make(chan struct{})}
			var once sync.Once
			release := func() { once.Do(func() { close(held.release) }) }
			defer release()
			runtime := activity.NewRuntimeService(activity.NewActivityService(svc.db, svc.settingsService), adapter)
			generation, err := runtime.InitializeGeneration(t.Context())
			require.NoError(t, err)
			store := activity.RuntimeStoreBinding{EnvironmentID: "0", StoreGeneration: generation, ExecutorGeneration: generation, ManagerID: "example-manager", RegistrationID: "example-registration"}
			require.NoError(t, runtime.ConfigureLocal(activity.RuntimeLocalConfig{ApplicationContext: t.Context(), Store: store, Executor: held}))
			admitted, err := runtime.Admit(t.Context(), activity.RuntimeCommand{Store: store, Key: uuid.NewString(), Actor: usertypes.Actor{ID: "example-owner", Username: "Operator"}, Operation: activity.RuntimeDownProject, Target: activity.RuntimeTarget{Kind: "stack", ID: target.ID}})
			require.NoError(t, err)
			select {
			case <-held.entered:
			case <-time.After(5 * time.Second):
				t.Fatal("executor barrier not reached")
			}
			reads.assertAndReset(t, "admission", target, true)
			switch change {
			case "renamed":
				identityRewriteSameMtime(t, filepath.Join(target.Path, "compose.yaml"), "name: changed_target\n"+identityServices)
			case "deleted":
				require.NoError(t, svc.db.Delete(target).Error)
			}
			release()
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			result, err := runtime.WaitLocal(ctx, activity.RuntimeRunReference{Store: store, RunID: admitted.ActivityID})
			require.NoError(t, err)
			reads.assertAndReset(t, "pre-effect", target, change != "deleted")
			if change == "unchanged" {
				require.NoError(t, held.err)
				require.Equal(t, activityDTO.StatusSuccess, result.Status)
				require.Equal(t, 1, port.effects)
			} else {
				code := "runtime.target-changed"
				if change == "deleted" {
					code = "runtime.target-unavailable"
				}
				require.ErrorContains(t, held.err, code)
				require.Equal(t, activityDTO.StatusFailed, result.Status)
				require.Zero(t, port.effects, "a stale target cannot reach dispatch")
			}
		})
	}
}

func TestRuntimeIdentityTargetModelMismatchStillRefuses(t *testing.T) {
	svc, root := runtimeIdentityFixture(t)
	target := runtimeIdentityProject(t, svc, root, "example-target", "target", "name: target_stack\n"+identityServices)
	load := runtimeLoadIdentity
	t.Cleanup(func() { runtimeLoadIdentity = load })
	calls := 0
	runtimeLoadIdentity = func(ctx context.Context, file, name, root string, inject bool) (*compose.Project, []string, error) {
		model, missing, err := load(ctx, file, name, root, inject)
		calls++
		if err == nil && calls == 2 {
			model.Name = "changed_target"
		}
		return model, missing, err
	}
	_, found, err := svc.ResolveRuntimeProjectIdentity(t.Context(), target.ID, "")
	require.ErrorContains(t, err, "project Compose identity differs between metadata and executable model")
	require.False(t, found)
	require.Equal(t, 2, calls)
}
