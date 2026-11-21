// Source: docklet-next/docs/design/stack-env-and-peers.md
// Revision: be6efa6e1470df78a90c7d485bc781be1206737a
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Real base-code encrypted baseline/point compatibility proof; M34 #548.

package lifecycle

import (
	"encoding/json"
	"path/filepath"
	"testing"
	"time"

	compose "github.com/compose-spec/compose-go/v2/types"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"go.getarcane.app/sys/crypto"
)

func TestRecordedSnapshotBasePayloadHashCompatibility(t *testing.T) {
	crypto.InitEncryption(&crypto.Config{Environment: "test", EncryptionKey: "synthetic-lifecycle-encryption-key-32bytes"})
	old := baseSnapshot{TargetID: "example-stack", TargetIdentity: Hash("identity"), RegistrationID: "registration", RunID: "base-run", Files: []File{{Path: "compose.yaml", Content: []byte("services: {}\n"), Present: true}}, Model: &compose.Project{Name: "example-stack", WorkingDir: "/srv/example/stack", ComposeFiles: []string{"/srv/example/stack/compose.yaml"}, Environment: compose.Mapping{"VALUE": "synthetic"}, Services: compose.Services{"app": {Name: "app", Image: "registry.example.com/app:1"}}}, Services: map[string]ServiceImage{}, Containers: map[string]ServiceImage{}}
	old.SavedRevision = Hash([]any{old.Files, old.Model})
	// baseSnapshot's MarshalJSON is the actual base implementation, independent
	// of today's Snapshot alias. Encrypt its bytes, then reopen with today's code.
	body, err := json.Marshal(old)
	require.NoError(t, err)
	require.NotContains(t, string(body), "External")
	baselinePayload, err := crypto.Encrypt(string(body))
	require.NoError(t, err)
	pointPayload, err := crypto.Encrypt(string(body))
	require.NoError(t, err)
	revision := Hash(old)
	raw := sqlitestore.URL(filepath.Join(t.TempDir(), "old-payloads.db"))
	db, err := database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	require.NoError(t, db.Create(&Baseline{TargetID: old.TargetID, TargetIdentity: old.TargetIdentity, RegistrationID: old.RegistrationID, SourceRunID: old.RunID, Revision: revision, Payload: baselinePayload, Available: true}).Error)
	require.NoError(t, db.Create(&Point{ID: "base-point", TargetID: old.TargetID, TargetIdentity: old.TargetIdentity, RegistrationID: old.RegistrationID, SourceRunID: old.RunID, Revision: revision, Payload: pointPayload, Available: true, CreatedAt: time.Now(), SourceState: "settled"}).Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), raw, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	store := Store{DB: db}
	current := &Snapshot{TargetID: old.TargetID, TargetIdentity: old.TargetIdentity, RegistrationID: old.RegistrationID}
	baseline, stored, err := store.Baseline(t.Context(), current)
	require.NoError(t, err)
	require.Empty(t, baseline.External)
	require.Equal(t, revision, stored)
	require.Equal(t, stored, Hash(baseline))
	point, saved, err := store.Point(t.Context(), "base-point")
	require.NoError(t, err)
	require.Equal(t, point.Revision, Hash(saved))
	require.Equal(t, old.SavedRevision, saved.SavedRevision)
	require.Equal(t, Hash([]any{saved.Files, saved.Model}), saved.SavedRevision)
	require.NoError(t, store.SaveBaseline(t.Context(), saved), "reopened old evidence remains usable")
	saved.External = []ExternalInput{{Path: "/srv/example/shared.env", SHA256: Hash("external")}}
	require.NotEqual(t, revision, Hash(saved))
}
