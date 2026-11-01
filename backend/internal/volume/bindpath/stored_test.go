// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package bindpath

import (
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/client"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestBindStoredRowsAdmitRemovedConsumers(t *testing.T) {
	db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
	require.NoError(t, err)
	for _, table := range []string{"volume_backup_policies", "volume_backups", "volume_restore_runs"} {
		require.NoError(t, db.Exec("CREATE TABLE "+table+" (volume_name TEXT)").Error)
	}
	require.NoError(t, db.Exec("CREATE TABLE backup_targets(kind TEXT,path TEXT,host_id TEXT)").Error)
	require.NoError(t, db.Exec("CREATE TABLE settings(key TEXT,value TEXT)").Error)
	pool, err := db.DB()
	require.NoError(t, err)
	t.Cleanup(func() { require.NoError(t, pool.Close()) })
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.HasSuffix(r.URL.Path, "/info") {
			_, _ = w.Write([]byte(`{"DockerRootDir":"/var/lib/docker"}`))
			return
		}
		require.True(t, strings.HasSuffix(r.URL.Path, "/containers/json"))
		_, _ = w.Write([]byte(`[]`))
	}))
	defer server.Close()
	cli, err := client.New(client.WithHost(server.URL), client.WithAPIVersion("1.41"))
	require.NoError(t, err)
	defer cli.Close()
	key := volume.BindKey("/srv/app")
	_, err = Check(t.Context(), cli, &database.DB{DB: db}, key)
	require.EqualError(t, err, "volume.bind.not-mounted")
	for _, table := range []string{"volume_backup_policies", "volume_backups", "volume_restore_runs"} {
		require.NoError(t, db.Exec("INSERT INTO "+table+" VALUES (?)", key).Error)
		source, err := Check(t.Context(), cli, &database.DB{DB: db}, key)
		require.NoError(t, err)
		require.Equal(t, "/srv/app", source)
		require.NoError(t, db.Exec("DELETE FROM "+table).Error)
	}
	require.NoError(t, db.Exec("INSERT INTO volume_backups VALUES (?)", key).Error)
	require.NoError(t, db.Exec("INSERT INTO backup_targets VALUES ('local','/srv/app','0')").Error)
	_, err = Check(t.Context(), cli, &database.DB{DB: db}, key)
	require.EqualError(t, err, "volume.bind.path-denied", "stored rows never bypass protection")
}
