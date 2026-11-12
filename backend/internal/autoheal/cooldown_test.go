// Source: docklet-next/issues/353
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original atomic cooldown and container identity guards.

package autoheal

import (
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func TestM19CooldownConcurrentReserveAndRecreatedID(t *testing.T) {
	path := sqlitestore.URL(filepath.Join(t.TempDir(), "cooldown.db"))
	first, err := database.Initialize(t.Context(), path, database.MigrationOptions{})
	require.NoError(t, err)
	defer first.Close()
	file, err := sqlitestore.Path(path)
	require.NoError(t, err)
	pool, err := sqlitestore.Open(t.Context(), file, true, false)
	require.NoError(t, err)
	defer pool.Close()
	g, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{})
	require.NoError(t, err)
	second := &database.DB{DB: g}
	now := time.Now().UTC()
	clock := func() time.Time { return now }
	stores := []*Store{{DB: first, Now: clock}, {DB: second, Now: clock}}
	cfg := dto.Settings{MaxRestarts: 1, WindowMinutes: 30, CooldownMinutes: 10}
	start := make(chan struct{})
	results := make(chan bool, 2)
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for i, s := range stores {
		wg.Go(func() {
			<-start
			row, restart, err := s.Reserve(t.Context(), []string{"scheduled", "recovery"}[i], "container", "web", "app", Baseline{}, cfg)
			if err == nil && !restart && row != nil {
				err = fmtError()
			}
			results <- restart
			errs <- err
		})
	}
	close(start)
	wg.Wait()
	require.NoError(t, <-errs)
	require.NoError(t, <-errs)
	a, b := <-results, <-results
	require.NotEqual(t, a, b)
	var count int64
	require.NoError(t, first.Model(&Restart{}).Count(&count).Error)
	require.EqualValues(t, 1, count)
	row, restart, err := stores[0].Reserve(t.Context(), "new-id", "replacement", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.True(t, restart)
	require.NotNil(t, row)
	now = now.Add(10 * time.Minute)
	row, restart, err = stores[0].Reserve(t.Context(), "after-cooldown", "container", "web", "app", Baseline{}, cfg)
	require.NoError(t, err)
	require.False(t, restart)
	require.Equal(t, "limit-reached", row.Outcome)
}
func fmtError() error { return errors.New("cooldown skip created a marker") }
