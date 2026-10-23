// Source: docklet-next/issues/193
// Revision: fdbb8c1fae620d9f97a485983cdaabb3fa53bca2
// Retrieved: 2026-10-23
// License: Apache-2.0
// Deviations: Original cancelled-create and nonblocking lock regressions.

package volume

import (
	"context"
	"fmt"
	"runtime"
	"strings"
	"testing"
	"time"

	workspacechild "github.com/florianfinn/docklet-next/backend/internal/volume/children/workspace"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDetachedHelperCreateCannotResurrectRemovedVolume(t *testing.T) {
	for _, operation := range []string{"prune", "rename"} {
		for _, request := range []string{"list", "file", "download", "update"} {
			for _, failedStart := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/failed-start=%t", operation, request, failedStart), func(t *testing.T) {
					f := &helperLifecycleFixture{imageStarted: make(chan struct{}), imageProceed: make(chan struct{})}
					svc := f.service(t)
					if failedStart {
						f.startStatus = 500
					}
					f.containers = nil
					svc.helperByVolume = map[string]*volumeHelper{}
					svc.workspace = workspacechild.NewService(workspacechild.Dependencies{Docker: svc.dockerService, Locks: &svc.workspaceLocks, AcquireHelper: svc.acquireVolumeHelperInternal})
					ctx, cancel := context.WithCancel(t.Context())
					defer cancel()
					done := make(chan error, 1)
					go func() {
						var err error
						switch request {
						case "list":
							_, err = svc.workspace.GetVolumeWorkspace(ctx, "data")
						case "file":
							_, err = svc.workspace.GetVolumeWorkspaceFile(ctx, "data", "a.txt")
						case "download":
							_, _, err = svc.workspace.DownloadVolumeWorkspaceFile(ctx, "data", "a.txt")
						case "update":
							_, err = svc.workspace.UpdateVolumeWorkspace(ctx, "data", volumetypes.WorkspaceUpdateManifest{FileTreeRevision: "r1", FileChanges: []volumetypes.WorkspaceFileChange{{Operation: volumetypes.FileOpCreateFolder, RelativePath: "new"}}}, nil, user.Actor{})
						}
						done <- err
					}()
					<-f.imageStarted
					// Join the actual singleflight completion before cancelling the only request.
					settled := svc.helperGroup.DoChan("data", func() (any, error) { panic("creation already in flight") })
					cancel()
					require.ErrorIs(t, <-done, context.Canceled)
					if operation == "prune" {
						result, err := svc.ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "data", CreatedAt: pruneCreated}})
						assert.NoError(t, err)
						assert.Equal(t, []volumetypes.PruneSkipped{{Name: "data", Reason: "in-use"}}, result.Skipped, "detached creation retains the volume lock")
					} else {
						_, err := svc.RenameVolume(t.Context(), "data", "renamed", user.Actor{})
						var busy *volumeWorkspaceBusyError
						assert.ErrorAs(t, err, &busy, "detached creation retains the volume lock")
					}
					close(f.imageProceed)
					result := <-settled
					if failedStart {
						require.ErrorContains(t, result.Err, "failed to start temp container")
					} else {
						require.NoError(t, result.Err)
					}
					// Completion includes registering the helper, not just sending a Docker create.
					require.Eventually(t, func() bool {
						unlock, ok := svc.workspaceLocks.TryLock("data")
						if ok {
							unlock()
						}
						return ok
					}, time.Second, time.Millisecond)
					assert.Empty(t, f.removed, "source must remain until the detached create settles")
					if len(f.removed) > 0 {
						assert.NotContains(t, f.volumes, "data", "detached helper creation resurrected the removed volume")
						return
					}
					require.Contains(t, f.volumes, "data")
					if operation == "prune" {
						result, err := svc.ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "data", CreatedAt: pruneCreated}})
						require.NoError(t, err)
						require.Equal(t, []string{"data"}, result.Removed)
					} else {
						_, err := svc.RenameVolume(t.Context(), "data", "renamed", user.Actor{})
						require.NoError(t, err)
					}
					require.NotContains(t, f.volumes, "data", "detached helper creation must not resurrect the removed volume")
				})
			}
		}
	}
}

func TestRenameWorkspaceLockIsNonblocking(t *testing.T) {
	f := &helperLifecycleFixture{}
	svc := f.service(t)
	unlock := svc.workspaceLocks.RLock("data")
	defer func() { unlock() }()
	done := make(chan error, 1)
	go func() { _, err := svc.RenameVolume(t.Context(), "data", "renamed", user.Actor{}); done <- err }()
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	for {
		select {
		case err := <-done:
			var busy *volumeWorkspaceBusyError
			require.ErrorAs(t, err, &busy, "rename must return busy without entering a blocking lock")
			return
		case <-timer.C:
			t.Fatal("rename neither returned nor reached the instrumented lock wait")
		default:
			stacks := make([]byte, 1<<20)
			stacks = stacks[:runtime.Stack(stacks, true)]
			for _, stack := range strings.Split(string(stacks), "\n\n") {
				if strings.Contains(stack, "(*KeyedMutex).Lock(") && strings.Contains(stack, "(*VolumeService).RenameVolume(") {
					// Release the observed blocking mutation, then fail an assertion;
					// a test timeout is not the behavioural regression guard.
					unlock()
					unlock = func() {}
					<-done
					require.FailNow(t, "rename entered a blocking workspace lock instead of returning busy")
				}
			}
			runtime.Gosched()
		}
	}
}
