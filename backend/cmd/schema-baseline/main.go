// Source: docklet-next/docs/design/core-adoption.md
// Revision: be847e36ab81fdf0cd6a7dad333046ddea990e71
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Original issue #8 baseline generation command.

package main

import (
	"errors"
	"fmt"
	"os"

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

func generate() error {
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		return errors.New("DATABASE_URL is required")
	}
	path, err := sqlitestore.Path(dsn)
	if err != nil {
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
	if err := db.AutoMigrate(schema.Models()...); err != nil {
		return err
	}
	return schema.ApplyIntegrity(db)
}
