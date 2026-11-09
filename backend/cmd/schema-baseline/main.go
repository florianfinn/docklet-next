// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Frozen release model, integrity and seed generation with deterministic SQL output and release identity.

package main

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/gofrs/flock"

	"context"
	"github.com/florianfinn/docklet-next/backend/internal/schema"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"gorm.io/gorm"
)

func main() {
	if err := generate(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func generate() error { return generateTo(os.Stdout) }

func generateTo(output io.Writer) error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	path, err := sqlitestore.Path(dsn)
	if err != nil {
		return err
	}
	if err := sqlitestore.ValidateFile(path + ".lock"); err != nil {
		return err
	}
	lock := flock.New(path+".lock", flock.SetPermissions(0600))
	defer lock.Close()
	locked, err := lock.TryLock()
	if err != nil {
		return err
	}
	if !locked {
		return errors.New("hub.already_running")
	}
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return fmt.Errorf("baseline generation requires a new database file: %w", err)
	}
	if err := file.Close(); err != nil {
		return err
	}
	pool, err := sqlitestore.Open(context.Background(), path, true, false)
	if err != nil {
		return err
	}
	defer pool.Close()
	db, err := gorm.Open(sqlitestore.Dialector(pool), &gorm.Config{IgnoreRelationshipsWhenMigrating: true})
	if err != nil {
		return err
	}
	var tables int
	if err := db.Raw("SELECT count(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'").Scan(&tables).Error; err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("baseline generation requires an empty database")
	}
	if err := db.AutoMigrate(schema.BaselineModels()...); err != nil {
		return err
	}
	if err := schema.ApplyBaselineIntegrity(db); err != nil {
		return err
	}
	rows, err := db.Raw("SELECT sql FROM sqlite_master WHERE sql IS NOT NULL AND name NOT LIKE 'sqlite_%' ORDER BY CASE type WHEN 'table' THEN 0 ELSE 1 END,name").Rows()
	if err != nil {
		return err
	}
	var statements []string
	for rows.Next() {
		var statement string
		if err := rows.Scan(&statement); err != nil {
			rows.Close()
			return err
		}
		statements = append(statements, statement+";")
	}
	if err := errors.Join(rows.Err(), rows.Close()); err != nil {
		return err
	}
	fmt.Fprint(output, baselineHeader)
	for _, statement := range statements {
		fmt.Fprintln(output, statement)
	}
	fmt.Fprintln(output, "CREATE TABLE docklet_database_metadata (identity TEXT PRIMARY KEY NOT NULL CHECK (identity = 'docklet-next:sqlite:2'));")
	fmt.Fprintln(output, "INSERT INTO docklet_database_metadata(identity) VALUES ('docklet-next:sqlite:2');")
	for _, seed := range baselineSeeds {
		fmt.Fprintln(output, seed)
	}
	return nil
}

// Only SQL seeds left by the prerelease chain belong in the release baseline.
var baselineSeeds = []string{
	`INSERT INTO settings(key,value) VALUES('updateNotify','false');`,
	`INSERT INTO settings(key,value) VALUES('vulnerabilityScanSchedule','{"mode":"manual"}');`,
}

const baselineHeader = `-- Source: docklet-next/docs/design/sqlite-database.md
-- Revision: b11b3e70e671dd3c862b966b1d279251842c7319
-- Retrieved: 2026-11-09
-- License: Apache-2.0 AND BSD-3-Clause
-- Deviations: Release SQLite baseline from frozen retained models, reviewed integrity overlay and SQL seeds; retired update-all table omitted.
-- +goose Up
`
