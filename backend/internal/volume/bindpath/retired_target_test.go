// Source: docklet-next/issues/388
// Revision: db966bd5e74458ea032d70eba1b7e4593c7b104e
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original active and retired stored-target path protection proofs.

package bindpath

import (
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/testsqlite"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"testing"
	"time"
)

func TestStoredTargetsProtectRetiredPaths(t *testing.T) {
	for _, host := range []string{"0", "agent"} {
		for _, kind := range []string{"local", "mount"} {
			for _, retired := range []bool{false, true} {
				name := host + "/" + kind + "/active"
				if retired {
					name = host + "/" + kind + "/retired"
				}
				t.Run(name, func(t *testing.T) {
					db, err := gorm.Open(testsqlite.Dialector(t, ":memory:"), &gorm.Config{})
					require.NoError(t, err)
					pool, err := db.DB()
					require.NoError(t, err)
					defer pool.Close()
					require.NoError(t, db.AutoMigrate(&backuptarget.Target{}))
					require.NoError(t, db.Exec("CREATE TABLE settings(key TEXT,value TEXT)").Error)
					if host != "0" {
						require.NoError(t, db.Exec("INSERT INTO settings VALUES ('hubHostId',?)", host).Error)
					}
					target := backuptarget.Target{ID: "target", Name: "Archive", Kind: kind, HostID: new(host), Path: "/srv/archive/repository", ConfigVersion: 1}
					if retired {
						target.RetiredAt = new(time.Now().UTC())
					}
					require.NoError(t, db.Create(&target).Error)
					foreign := target
					foreign.ID = "foreign"
					foreign.HostID = new("other")
					foreign.Path = "/srv/foreign"
					require.NoError(t, db.Create(&foreign).Error)
					shared := target
					shared.ID = "shared"
					shared.Kind = "nfs"
					shared.HostID = nil
					shared.Path = "/srv/shared"
					require.NoError(t, db.Create(&shared).Error)
					paths, err := TargetPaths(t.Context(), &database.DB{DB: db})
					require.NoError(t, err)
					for _, source := range []string{"/srv/archive", "/srv/archive/repository", "/srv/archive/repository/data"} {
						for _, stored := range []bool{false, true} {
							err := Rules(source, "", nil, paths, stored)
							var problem *dto.Problem
							require.ErrorAs(t, err, &problem, "stored target paths stay protected after retirement")
							require.Equal(t, "volume.bind.path-denied", problem.Code)
							require.Equal(t, "docklet", problem.Reason)
						}
					}
					require.NoError(t, Rules("/srv/foreign", "", nil, paths, true))
					require.NoError(t, Rules("/srv/shared", "", nil, paths, true))
					require.NoError(t, Rules("/srv/archive/repository-other", "", nil, paths, true))
				})
			}
		}
	}
}
