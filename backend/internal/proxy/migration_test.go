// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original 004 to 005 upgrade and stable journal generation proof.

package proxy

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/florianfinn/docklet-next/backend/resources"
	dto "github.com/florianfinn/docklet-next/types/proxy"
	"github.com/stretchr/testify/require"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func TestP20Upgrade004PreservesUnrelatedRows(t *testing.T) {
	source, err := fs.Sub(resources.FS, "migrations/sqlite")
	require.NoError(t, err)
	entries, err := fs.ReadDir(source, ".")
	require.NoError(t, err)
	old := fstest.MapFS{}
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "005_") {
			continue
		}
		b, err := fs.ReadFile(source, e.Name())
		require.NoError(t, err)
		old[e.Name()] = &fstest.MapFile{Data: b}
	}
	url := sqlitestore.URL(filepath.Join(t.TempDir(), "upgrade.db"))
	db, err := database.Initialize(t.Context(), url, database.MigrationOptions{Source: old})
	require.NoError(t, err)
	require.NoError(t, db.Exec("INSERT INTO settings(key,value) VALUES (?,?)", "example-preserved", "kept").Error)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	var value, generation string
	require.NoError(t, db.Table("settings").Select("value").Where("key = ?", "example-preserved").Scan(&value).Error)
	require.Equal(t, "kept", value)
	require.NoError(t, db.Table("proxy_generation").Select("generation").Scan(&generation).Error)
	require.NotEmpty(t, generation)
	require.NoError(t, db.Close())
	db, err = database.Initialize(t.Context(), url, database.MigrationOptions{})
	require.NoError(t, err)
	defer db.Close()
	a, err := NewAgent(db.DB, nil, nil)
	require.NoError(t, err)
	require.Equal(t, generation, a.Generation())
}

func TestP20PersistedIntentRoundtrip(t *testing.T) {
	for _, phase := range []dto.Phase{dto.Admitted, dto.Staging, dto.Publishing, dto.Reloading, dto.Restarting, dto.Observing, dto.Marking, dto.Restoring} {
		t.Run(string(phase), func(t *testing.T) {
			x := newFixture(t)
			digest, err := Digest(x.request)
			require.NoError(t, err)
			i := Intent{Request: x.request, Old: Snapshot([]byte("old"), true), New: Snapshot(nil, false), Mode: 0640, GraphHashes: x.request.GraphHashes, TempPath: ".docklet-test.tmp", MarkerPath: "docklet-check-test.yml", StagingPath: ".docklet-staging-test", Reload: true, Restart: true, StartedBefore: initialEpoch, Owned: true}
			o := dto.Operation{ID: x.request.ID, InstanceID: x.request.Instance.ID, Generation: x.a.Generation(), Registration: x.request.Registration, Digest: digest, State: dto.Pending, Phase: phase, CreatedAt: time.Now().UTC()}
			x.persist(t, i, o)
			restarted, err := NewAgent(x.db.DB, x.f, x.a.Engines)
			require.NoError(t, err)
			require.Equal(t, x.a.Generation(), restarted.Generation())
			j, err := findJournal(t.Context(), x.db.DB, o.ID)
			require.NoError(t, err)
			intent, result, err := decodeJournal(j)
			require.NoError(t, err)
			expected, er := marshal(i)
			require.NoError(t, er)
			actual, er := marshal(intent)
			require.NoError(t, er)
			require.JSONEq(t, string(expected), string(actual))
			require.Equal(t, o, result)
		})
	}
}
