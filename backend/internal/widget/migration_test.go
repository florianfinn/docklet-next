// Source: docklet-next/issues/332
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original widget persistence and restart checks; M21 runs them on the release baseline.

package widget

import (
	"io/fs"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/resources"
	dto "github.com/florianfinn/docklet-next/types/widget"
	"github.com/pressly/goose/v3"
	"github.com/stretchr/testify/require"
)

func TestSW12BaselinePersistsWidgetsAcrossRestart(t *testing.T) {
	s := testService(t)
	require.NoError(t, s.db.Migrator().DropTable(&row{}))
	sqlDB, err := s.db.DB.DB()
	require.NoError(t, err)
	migrations, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	provider, err := goose.NewProvider(goose.DialectSQLite3, sqlDB, migrations)
	require.NoError(t, err)
	_, err = provider.Up(t.Context())
	require.NoError(t, err)
	r := row{Kind: "plex", Enabled: true, Address: "https://plex.example.com", Revision: 1, UpdatedAt: time.Now().UTC()}
	require.NoError(t, save(t.Context(), s.db, r))
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
}
