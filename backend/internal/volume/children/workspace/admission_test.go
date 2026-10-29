// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original workspace write contention against backup ownership.

package workspace

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/require"
)

func TestWorkspaceWriteRefusesOwnedVolumeWithoutWaiting(t *testing.T) {
	inspected := make(chan struct{}, 1)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == "GET" {
			_, _ = w.Write([]byte(`{"Name":"app-data","Options":{}}`))
			inspected <- struct{}{}
			return
		}
		t.Error("busy workspace performed a mutation")
		w.WriteHeader(500)
	}))
	t.Cleanup(server.Close)
	cli := newVolumeWorkspaceTestDockerClientInternal(t, server)
	locks := &utils.KeyedMutex{}
	release := locks.Lock("app-data")
	service := NewService(Dependencies{Docker: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(cli), Locks: locks, BusyError: func(string) error { return errors.New("volume workspace is busy") }, AcquireHelper: func(context.Context, string) (string, func(), error) {
		t.Error("busy workspace acquired helper")
		return "", func() {}, errors.New("unexpected helper")
	}})
	result := make(chan error, 1)
	go func() {
		_, err := service.UpdateVolumeWorkspace(t.Context(), "app-data", volume.WorkspaceUpdateManifest{FileTreeRevision: "revision", FileChanges: []volume.WorkspaceFileChange{{Operation: volume.FileOpDelete, RelativePath: "old.txt"}}}, nil, user.SystemUser)
		result <- err
	}()
	<-inspected
	select {
	case err := <-result:
		release()
		require.ErrorIs(t, err, common.ErrVolumeWorkspaceConflict)
		require.Equal(t, 409, volumeWorkspaceHTTPErrorInternal(err).(interface{ GetStatus() int }).GetStatus())
	case <-time.After(time.Second):
		release()
		err := <-result
		require.ErrorIs(t, err, common.ErrVolumeWorkspaceConflict, "blocking admission executed after backup released ownership")
	}
}
