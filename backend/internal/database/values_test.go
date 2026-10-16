// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Driver-native timestamp, auto-time, NULL, JSON type and replacement-connection proof.

package database

import (
	"database/sql/driver"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"github.com/stretchr/testify/require"
	"path/filepath"
	"testing"
	"time"
)

type valueRow struct {
	ID        int `gorm:"primaryKey"`
	CreatedAt time.Time
	UpdatedAt time.Time
	ExpiresAt time.Time
	Nullable  *time.Time
	Zero      time.Time
	JSON      JSON        `gorm:"type:text"`
	Names     StringSlice `gorm:"type:text"`
}

func TestSQLiteValuesAndReplacementConnections(t *testing.T) {
	db, err := connectDatabaseInternal(t.Context(), sqlitestore.URL(filepath.Join(t.TempDir(), "values.db")))
	require.NoError(t, err)
	defer db.Close()
	require.NoError(t, db.AutoMigrate(&valueRow{}))
	now := time.Now().UTC().Truncate(time.Microsecond)
	future := now.Add(time.Hour)
	row := valueRow{ExpiresAt: future, JSON: JSON{"v": true}, Names: StringSlice{}}
	require.NoError(t, db.Create(&row).Error)
	require.False(t, row.CreatedAt.IsZero())
	require.False(t, row.UpdatedAt.IsZero())
	row.CreatedAt = now
	require.NoError(t, db.Save(&row).Error)
	require.NoError(t, db.Model(&row).Updates(map[string]any{"expires_at": future, "nullable": now}).Error)
	var count int64
	require.NoError(t, db.Model(&valueRow{}).Where("expires_at < ?", now).Count(&count).Error)
	require.Zero(t, count, "future values must never expire through INTEGER/TEXT comparison")
	var loaded valueRow
	require.NoError(t, db.First(&loaded, row.ID).Error)
	require.True(t, loaded.ExpiresAt.Equal(future))
	require.True(t, loaded.Zero.IsZero())
	require.NotNil(t, loaded.Nullable)
	require.True(t, loaded.Nullable.Equal(now))
	require.Equal(t, time.UTC, loaded.ExpiresAt.Location())
	require.Equal(t, row.JSON, loaded.JSON)
	require.Empty(t, loaded.Names)
	for _, column := range []string{"created_at", "updated_at", "expires_at", "nullable", "zero"} {
		var typ string
		require.NoError(t, db.Raw("SELECT typeof("+column+") FROM value_rows WHERE id=?", row.ID).Scan(&typ).Error)
		require.Equal(t, "integer", typ, column)
	}
	for _, column := range []string{"json", "names"} {
		var typ string
		require.NoError(t, db.Raw("SELECT typeof("+column+") FROM value_rows WHERE id=?", row.ID).Scan(&typ).Error)
		require.Equal(t, "text", typ, column)
	}
	require.NoError(t, db.Model(&row).Update("nullable", nil).Error)
	loaded = valueRow{}
	require.NoError(t, db.First(&loaded, row.ID).Error)
	require.Nil(t, loaded.Nullable)
	var micro int64
	require.NoError(t, db.Raw("SELECT MAX(expires_at) FROM value_rows").Scan(&micro).Error)
	require.Equal(t, future.UnixMicro(), micro)
	pool, err := db.SQLDB()
	require.NoError(t, err)
	conn, err := pool.Conn(t.Context())
	require.NoError(t, err)
	err = conn.Raw(func(any) error { return driver.ErrBadConn })
	require.ErrorIs(t, err, driver.ErrBadConn)
	_ = conn.Close()
	require.NoError(t, sqlitestore.Verify(t.Context(), pool))
	require.NoError(t, db.Model(&valueRow{}).Where("expires_at < ?", now).Count(&count).Error)
	require.Zero(t, count)
	var rebound time.Time
	require.NoError(t, db.Raw("SELECT expires_at FROM value_rows WHERE id=?", row.ID).Row().Scan(&rebound))
	require.True(t, rebound.Equal(future))
	require.NoError(t, db.Exec("CREATE TABLE children(parent INTEGER REFERENCES value_rows(id))").Error)
	require.Error(t, db.Exec("INSERT INTO children(parent) VALUES(-1)").Error)
}
