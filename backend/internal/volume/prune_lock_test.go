// Source: docklet-next/docs/design/storage-workspace.md
// Revision: ce3c4ea4af5b0bc1cdb2bb287d30281bbe20f0a8
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original R1 regression coverage for issue #179.
package volume

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/event"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/user"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestPruneSerializesWithRename(t *testing.T) {
	inspected, proceed := make(chan struct{}), make(chan struct{})
	var removed atomic.Bool
	var inspectCount atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/system/df"):
			_, _ = w.Write([]byte(`{"Volumes":[]}`))
		case strings.HasSuffix(r.URL.Path, "/containers/json"):
			_, _ = w.Write([]byte(`[]`))
		case strings.HasSuffix(r.URL.Path, "/volumes/source"):
			if r.Method == "DELETE" {
				removed.Store(true)
				w.WriteHeader(204)
				return
			}
			if inspectCount.Add(1) == 1 {
				close(inspected)
				<-proceed
			}
			if removed.Load() {
				http.Error(w, `{"message":"removed"}`, 404)
				return
			}
			_ = json.NewEncoder(w).Encode(pruneVolume("source"))
		default:
			t.Errorf("unexpected engine request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(500)
		}
	}))
	defer server.Close()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&event.Event{}))
	svc := &VolumeService{eventService: event.NewEventService(&database.DB{DB: db}, nil, nil), dockerService: docker.NewDockerClientService(t.Context(), nil, nil, nil).WithClient(newVolumeServiceTestDockerClientInternal(t, server)), helperByVolume: map[string]*volumeHelper{}}
	pruneDone := make(chan error, 1)
	go func() {
		_, err := svc.ExecuteVolumePrune(t.Context(), []volumetypes.PruneCandidate{{Name: "source", CreatedAt: pruneCreated}})
		pruneDone <- err
	}()
	select {
	case <-inspected:
	case <-time.After(5 * time.Second):
		t.Fatal("prune inspect barrier")
	}
	unlock, acquired := svc.workspaceLocks.TryLock("source")
	if acquired {
		unlock()
	}
	assert.False(t, acquired, "prune must own the workspace lock before inspecting/removing the rename source")
	if acquired {
		close(proceed)
		require.NoError(t, <-pruneDone)
		return
	}
	renameDone := make(chan error, 1)
	go func() { _, err := svc.RenameVolume(t.Context(), "source", "target", user.Actor{}); renameDone <- err }()
	close(proceed)
	require.NoError(t, <-pruneDone)
	require.ErrorContains(t, <-renameDone, "inspect source volume", "rename observes the removal after acquiring the shared lock")
	require.True(t, removed.Load())
}
