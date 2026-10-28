// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original async target-test admission, configuration freshness and safe diagnostics proof.

package backup

import (
	"context"
	"errors"
	"net/http"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/activity"
	"github.com/florianfinn/docklet-next/backend/internal/backup"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/types/user"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

type checkSetting struct {
	Key   string `gorm:"primaryKey"`
	Value string
}

func (checkSetting) TableName() string { return "settings" }
func TestTargetCheckAdmissionDoesNotMountAndKeepsAdmittedConfig(t *testing.T) {
	dockerEffects := 0
	restarted := []string{}
	s := startupService(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		dockerEffects++
		t.Errorf("test performed Docker effects before helper resolution: %s", r.URL.Path)
		w.WriteHeader(500)
	}), &restarted)
	require.NoError(t, s.deps.DB.AutoMigrate(activity.Models()...))
	require.NoError(t, s.deps.DB.AutoMigrate(&checkSetting{}, &backuptarget.Target{}, &backuptarget.Check{}))
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-encryption-key-at-least-32-bytes"})
	key, err := backup.GenerateRecoveryKey()
	require.NoError(t, err)
	encrypted, err := crypto.Encrypt(key)
	require.NoError(t, err)
	host := "0"
	require.NoError(t, s.deps.DB.Create(&backuptarget.Target{ID: "target", Name: "Archive", Kind: "local", HostID: &host, Path: "/srv/backup", RecoveryKeyEnc: encrypted, ConfigVersion: 7, CreatedAt: time.Now().UTC()}).Error)
	s.deps.Activity = activity.NewActivityService(s.deps.DB, nil)
	events, _, unsubscribe := s.deps.Activity.Subscribe("0")
	defer unsubscribe()
	s.deps.Engine = backup.NewEngine(t.Context(), newVolumeAdmissionForTestInternal(t), nil)
	helperEntered := make(chan context.Context, 1)
	release := make(chan struct{})
	s.deps.HelperImage = func(ctx context.Context, _ *client.Client) (string, error) {
		helperEntered <- ctx
		select {
		case <-release:
		case <-ctx.Done():
		}
		return "", errors.New("synthetic private diagnostic sentinel")
	}
	ctx := context.WithValue(t.Context(), user.CurrentUserContextKey{}, &user.SystemUser)
	requestCtx, requestCancel := context.WithCancel(ctx)
	handler := TargetHandler(s, nil, nil, t.Context())
	accepted, err := handler.Test(requestCtx, "target", "0")
	require.NoError(t, err)
	require.NotEmpty(t, accepted.ActivityID)
	workerCtx := <-helperEntered
	requestCancel()
	require.ErrorIs(t, requestCtx.Err(), context.Canceled)
	require.NoError(t, workerCtx.Err(), "accepted target test must survive its HTTP request")
	require.Zero(t, dockerEffects, "test admission must not mount a target")
	var admitted backuptarget.Check
	require.NoError(t, s.deps.DB.First(&admitted, "target_id = ?", "target").Error)
	require.Equal(t, int64(7), admitted.ConfigVersion)
	require.Equal(t, accepted.ActivityID, admitted.ActivityID)
	require.NoError(t, s.deps.DB.Model(&backuptarget.Target{}).Where("id = ?", "target").Update("config_version", 8).Error)
	close(release)
	wait, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	for {
		select {
		case event := <-events:
			if event.Activity != nil && event.Activity.ID == accepted.ActivityID && (event.Activity.Status == "failed" || event.Activity.Status == "success" || event.Activity.Status == "cancelled") {
				goto completed
			}
		case <-wait.Done():
			t.Fatal("repository check did not publish its terminal activity")
		}
	}
completed:
	var completed backuptarget.Check
	require.NoError(t, s.deps.DB.First(&completed, "target_id = ?", "target").Error)
	require.Equal(t, int64(7), completed.ConfigVersion)
	require.False(t, completed.OK)
	require.Equal(t, "volume.backup.repository-check-failed", completed.Code)
	require.Nil(t, completed.FreeBytes)
	require.Nil(t, completed.TotalBytes)
	var recorded activity.Activity
	require.NoError(t, s.deps.DB.First(&recorded, "id = ?", accepted.ActivityID).Error)
	require.EqualValues(t, "failed", recorded.Status)
	require.NotContains(t, recorded.LatestMessage, "sentinel")
}
