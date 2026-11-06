// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original competing SQLite insert and atomic event-prune evidence.

package event

import (
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"testing"
	"time"
)

func TestM19ConcurrentPruneAndInsert(t *testing.T) {
	for _, insertFirst := range []bool{false, true} {
		t.Run(map[bool]string{false: "after-prune", true: "before-prune"}[insertFirst], func(t *testing.T) {
			path := sqlitestore.URL(filepath.Join(t.TempDir(), "prune.db"))
			db, err := database.Initialize(t.Context(), path, database.MigrationOptions{})
			require.NoError(t, err)
			defer db.Close()
			file, err := sqlitestore.Path(path)
			require.NoError(t, err)
			pool, err := sqlitestore.Open(t.Context(), file, true, false)
			require.NoError(t, err)
			defer pool.Close()
			g, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{})
			require.NoError(t, err)
			for i := range 60 {
				require.NoError(t, db.Exec("INSERT INTO events(id,type,title,timestamp) VALUES(?,?,?,?)", fmt.Sprintf("old-%03d", i), "container.start", "Example", time.Now().Add(-time.Hour+time.Duration(i)*time.Second)).Error)
			}
			tx := db.Begin()
			require.NoError(t, tx.Error)
			inserted := func(conn *gorm.DB) error {
				return conn.Exec("INSERT INTO events(id,type,title,timestamp) VALUES('new','container.start','Example',?)", time.Now()).Error
			}
			done := make(chan error, 1)
			start := make(chan struct{})
			if insertFirst {
				require.NoError(t, inserted(tx))
				go func() {
					close(start)
					_, err := NewEventService(&database.DB{DB: g}, nil, nil).PruneEvents(t.Context(), 50)
					done <- err
				}()
			} else {
				_, err := NewEventService(&database.DB{DB: tx}, nil, nil).PruneEvents(t.Context(), 50)
				require.NoError(t, err)
				go func() { close(start); done <- inserted(g) }()
			}
			<-start
			require.NoError(t, tx.Commit().Error)
			require.NoError(t, <-done)
			var count int64
			require.NoError(t, db.Model(&Event{}).Where("id='new'").Count(&count).Error)
			require.EqualValues(t, 1, count)
			require.NoError(t, db.Model(&Event{}).Count(&count).Error)
			want := 51
			if insertFirst {
				want = 50
			}
			require.EqualValues(t, want, count)
		})
	}
}
