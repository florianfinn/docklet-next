// Source: docklet-next/docs/design/sqlite-database.md
// Revision: b3607be5144101152c8eb7738d389e8c73986fba
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original pre-migration ownership and forward-history validation.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
)

const identity = "docklet-next:sqlite:1"

func validateIdentity(ctx context.Context, db *sql.DB, required int64) error {
	rows, err := db.QueryContext(ctx, "SELECT name FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'")
	if err != nil {
		return fmt.Errorf("inspect SQLite schema: %w", err)
	}
	tables := map[string]bool{}
	for rows.Next() {
		var name string
		if err = rows.Scan(&name); err != nil {
			break
		}
		tables[name] = true
	}
	err = errors.Join(err, rows.Err(), rows.Close())
	if err != nil {
		return err
	}
	if len(tables) == 0 {
		return nil
	}
	foreign := errors.New("foreign or inconsistent database; start with an empty Docklet SQLite database")
	if tables[legacyVersionTable] || !tables["docklet_database_metadata"] || !tables["goose_db_version"] {
		return foreign
	}
	var owner string
	var count int
	if err = db.QueryRowContext(ctx, "SELECT count(*),coalesce(max(identity),'') FROM docklet_database_metadata").Scan(&count, &owner); err != nil {
		return foreign
	}
	if count != 1 || owner != identity {
		return foreign
	}
	rows, err = db.QueryContext(ctx, "SELECT version_id,is_applied FROM goose_db_version ORDER BY id")
	if err != nil {
		return foreign
	}
	defer rows.Close()
	versions, err := getEmbeddedMigrationVersionsInternal(dbProviderSQLite)
	if err != nil {
		return err
	}
	index := -1
	for rows.Next() {
		var version int64
		var applied bool
		if err = rows.Scan(&version, &applied); err != nil {
			return err
		}
		if version > required {
			return fmt.Errorf("database schema version %d is newer than supported version %d", version, required)
		}
		if !applied {
			return foreign
		}
		expected := int64(0)
		if index >= 0 {
			if index >= len(versions) {
				return foreign
			}
			expected = versions[index]
		}
		if version != expected {
			return foreign
		}
		index++
	}
	if err = rows.Err(); err != nil {
		return err
	}
	if index < 1 {
		return foreign
	}
	return nil
}
