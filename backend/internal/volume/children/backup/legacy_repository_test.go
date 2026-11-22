// Source: docklet-next/issues/570
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original retained repository mount, key and snapshot-list proof through the Rustic engine.; M36 #570: Docklet backup volume and runtime names.

package backup

import (
	"context"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	"github.com/stretchr/testify/require"
	kit "go.getarcane.app/kit/pkg"
)

func TestLegacyRepositoryRetainsSnapshotsAndOriginalKey(t *testing.T) {
	f := newRuntimePlan(t)
	snapshot := strings.Repeat("a", 64)
	f.stdout = `[{"id":"` + snapshot + `"}]`
	f.volumeState = map[string]map[string]string{"arcane-backups": {}}
	resolved := 0
	f.service.deps.ResolveBackupVolume = func(context.Context) (string, error) { resolved++; return "arcane-backups", nil }
	f.service.deps.EncryptionKey = "synthetic-encryption-key-at-least-32-bytes"
	cli, err := f.service.deps.Docker.GetClient(t.Context())
	require.NoError(t, err)
	repository, err := f.service.localRusticRepositoryInternal(t.Context(), cli, true)
	require.NoError(t, err)
	password, err := f.service.volumeBackupPasswordInternal(t.Context(), cli)
	require.NoError(t, err)
	original := kit.SHA256Hex("arcane-volume-backups:" + f.service.deps.EncryptionKey)
	require.Equal(t, original, password)
	snapshots, err := f.service.deps.Engine.ListSnapshots(t.Context(), cli, repository, password)
	require.NoError(t, err)
	require.Len(t, snapshots, 1)
	require.Equal(t, snapshot, snapshots[0].ID)
	require.Equal(t, 1, resolved)
	creates, _, _ := f.effects()
	found := false
	for _, created := range creates {
		if created.Image != rustic.DefaultImage {
			continue
		}
		found = true
		require.Contains(t, created.Env, "RUSTIC_PASSWORD="+original)
		require.Contains(t, created.Env, "RUSTIC_REPOSITORY=/repository/volumes")
		require.Equal(t, "arcane-backups", created.HostConfig.Mounts[0].Source)
	}
	require.True(t, found, "the original repository must be opened by the real engine path")
}
