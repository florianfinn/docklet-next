// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original forward widget migration and persisted restart checks.

package widget

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io/fs"
	"os"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/resources"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestSW12ForwardMigrationFromPreviousSchema(t *testing.T) {
	s := testService(t)
	require.NoError(t, s.db.Migrator().DropTable(&row{}))
	sqlDB, err := s.db.DB.DB()
	require.NoError(t, err)
	migrations, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 15)
	require.NoError(t, err)
	_, err = provider.UpTo(t.Context(), 17)
	require.NoError(t, err)
	r := row{Kind: "plex", Enabled: true, Address: "https://plex.example.com", Revision: 1, UpdatedAt: time.Now().UTC()}
	require.NoError(t, save(t.Context(), s.db, r))
	_, err = provider.UpTo(t.Context(), 17)
	require.NoError(t, err)
	got, err := load(t.Context(), s.db, "plex")
	require.NoError(t, err)
	require.Equal(t, r.Address, got.Address)
	require.EqualValues(t, 1, got.Revision)
	for _, kind := range []dto.Kind{dto.Plex, dto.Overseerr, dto.Minecraft} {
		copy := r
		copy.Kind = kind
		require.NoError(t, save(t.Context(), s.db, copy))
	}
	_, err = s.Save(t.Context(), dto.Plex, dto.ConfigUpdate{Enabled: true, Address: r.Address, CredentialAction: "replace", Credential: "migration-synthetic-token"})
	require.NoError(t, err)
	s = reopen(t, s)
	persisted, err := load(t.Context(), s.db, dto.Plex)
	require.NoError(t, err)
	require.NotNil(t, persisted.CredentialEnc)
	require.NotContains(t, *persisted.CredentialEnc, "migration-synthetic-token")
	value, err := secret(persisted)
	require.NoError(t, err)
	require.Equal(t, "migration-synthetic-token", value)
	r.Kind = "invalid"
	require.Error(t, save(t.Context(), s.db, r), "SQL kind CHECK must constrain external writes")
	bytes, err := fs.ReadFile(migrations, "017_service_widgets.sql")
	require.NoError(t, err)
	manifestBytes, err := os.ReadFile("../../../scripts/migration-hashes.json")
	require.NoError(t, err)
	var manifest map[string]string
	require.NoError(t, json.Unmarshal(manifestBytes, &manifest))
	sum := sha256.Sum256(bytes)
	require.Equal(t, manifest["sqlite/017_service_widgets.sql"], hex.EncodeToString(sum[:]))
}
