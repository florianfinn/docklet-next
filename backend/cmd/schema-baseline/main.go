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

	"github.com/florianfinn/docklet-next/backend/internal/schema"
	"gorm.io/driver/postgres"
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
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{IgnoreRelationshipsWhenMigrating: true})
	if err != nil {
		return err
	}
	var tables int
	if err := db.Raw("SELECT count(*) FROM pg_tables WHERE schemaname = 'public'").Scan(&tables).Error; err != nil {
		return err
	}
	if tables != 0 {
		return errors.New("baseline generation requires an empty database")
	}
	return db.AutoMigrate(schema.Models()...)
}
