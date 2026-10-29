// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original deadline and confirmed mount-helper ownership proof.

package backup

import (
	"context"
	"net/http"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestTargetTestDeadlineKeepsLeaseAndMaintenanceUntilHelperAbsenceConfirmed(t *testing.T) {
	entered, expired, inspected := make(chan struct{}, 1), make(chan struct{}, 1), make(chan struct{}, 1)
	var confirmed atomic.Bool
	restarted := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/containers/create"):
			_, _ = w.Write([]byte(`{"Id":"held-mount"}`))
		case strings.HasSuffix(r.URL.Path, "/containers/held-mount/start"):
			entered <- struct{}{}
			<-r.Context().Done()
			expired <- struct{}{}
		case strings.HasSuffix(r.URL.Path, "/containers/held-mount") && r.Method == http.MethodDelete:
			w.WriteHeader(204)
		case strings.HasSuffix(r.URL.Path, "/containers/held-mount/json"):
			select {
			case inspected <- struct{}{}:
			default:
			}
			if confirmed.Load() {
				w.WriteHeader(404)
				_, _ = w.Write([]byte(`{"message":"not found"}`))
			} else {
				w.WriteHeader(503)
				_, _ = w.Write([]byte(`{"message":"still mounted"}`))
			}
		default:
			t.Errorf("unexpected effect %s", r.URL.Path)
			w.WriteHeader(500)
		}
	}), &restarted)
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	key, err := backup.GenerateRecoveryKey()
	require.NoError(t, err)
	encrypted, err := crypto.Encrypt(key)
	require.NoError(t, err)
	host := "0"
	require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "archive", Kind: "local", HostID: &host, Name: "Archive", Path: "/srv/backups", RecoveryKeyEnc: encrypted, ConfigVersion: 1, CreatedAt: time.Now().UTC()}).Error)
	s.deps.Engine = backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	s.deps.Activity = activity.NewActivityService(s.deps.DB, nil)
	s.deps.HelperImage = func(context.Context, *client.Client) (string, error) { return volumehelper.DefaultToolsImage, nil }
	app, cancel := context.WithTimeout(t.Context(), time.Second)
	defer cancel()
	handler := TargetHandler(s, nil, nil, app)
	ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
	accepted, err := handler.Test(ctx, "archive", "0")
	require.NoError(t, err)
	require.NotEmpty(t, accepted.ActivityID)
	select {
	case <-entered:
	case <-time.After(2 * time.Second):
		t.Fatal("mount start did not begin")
	}
	select {
	case <-expired:
	case <-time.After(2 * time.Second):
		t.Fatal("mount request did not end on deadline")
	}
	select {
	case <-inspected:
	case <-time.After(2 * time.Second):
		t.Fatal("helper cleanup never inspected")
	}
	acquiredDuringCleanup := func() bool {
		lease, ok, err := s.deps.Engine.TryAcquireRun(t.Context(), backup.VolumeAdmissionScope, "repository:archive")
		require.NoError(t, err)
		if ok {
			lease.Release(t.Context())
		}
		return ok
	}
	require.Never(t, acquiredDuringCleanup, 150*time.Millisecond, 10*time.Millisecond, "deadline cannot release a surviving mount helper")
	drained := make(chan error, 1)
	go func() { drained <- s.deps.Activity.CancelAndDrain(t.Context(), "") }()
	select {
	case <-drained:
		t.Error("maintenance passed a surviving helper")
	case <-time.After(50 * time.Millisecond):
	}
	confirmed.Store(true)
	select {
	case err := <-drained:
		require.NoError(t, err)
	case <-time.After(3 * time.Second):
		t.Fatal("confirmed cleanup did not settle maintenance")
	}
	require.True(t, acquiredDuringCleanup())
}
