// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original image and registry safety contract for issue #229.

package prune

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/image/guard"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	francistest "github.com/florianfinn/docklet-next/backend/pkg/utils/francis/testing"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"
)

func TestPR1ScheduledAndInteractivePruneShareImageGuard(t *testing.T) {
	id := "sha256:" + strings.Repeat("a", 64)
	var deletes atomic.Int32
	daemon := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.Method == http.MethodDelete {
			deletes.Add(1)
			_, _ = fmt.Fprint(w, `[]`)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/images/json") {
			_, _ = fmt.Fprintf(w, `[{"Id":%q}]`, id)
			return
		}
		_, _ = fmt.Fprintf(w, `{"Id":%q}`, id)
	}))
	defer daemon.Close()
	c, err := client.New(client.WithHost(daemon.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer c.Close()
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&guard.Protection{}))
	runtime := francistest.New(t)
	admission := runs.NewAdmission(runtime.Service(), t.Name())
	require.NoError(t, admission.Register(runtime))
	francistest.Start(t, runtime)
	getter := func(context.Context) (*client.Client, error) { return c, nil }
	g := &guard.Service{DB: &database.DB{DB: db}, Docker: getter, Admission: admission}
	require.NoError(t, g.Protect(t.Context(), guard.Protection{ImageID: id, Reason: "restore-point", OwnerID: "point"}))
	s := NewService(time.UTC, getter, nil, nil, nil, nil)
	s.SetImageGuard(g)
	err = s.removePruneResourceInternal(t.Context(), c, pruneResourceInternal{Kind: "image", ID: id})
	assert.Equal(t, "image.restore-point-protected", imageDTO.Neutral(err, "unexpected", 500).Problem.Code)
	require.Zero(t, deletes.Load())
	result := &system.PruneAllResult{}
	require.NoError(t, s.pruneImagesInternal(t.Context(), system.PruneImagesOptions{Mode: system.PruneImageModeAll}, result))
	require.Equal(t, []imageDTO.PruneItem{{ID: id, Code: "image.restore-point-protected"}}, result.ImageSkipped)
	require.Zero(t, deletes.Load())
}
