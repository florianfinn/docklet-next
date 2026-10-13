// Source: getarcaneapp/arcane/backend/internal/database/database.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; PostgreSQL-only forward migrations, without legacy adoption or downgrades.

package database

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/pressly/goose/v3"
	"github.com/pressly/goose/v3/lock"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/florianfinn/docklet-next/backend/resources"
)

type DB struct {
	*gorm.DB
}

type MigrationOptions struct{}

const (
	dbProviderPostgres = "postgres"
	legacyVersionTable = "schema_migrations"

	// Prepared-statement cache bounds. GORM's PrepareStmt cache is a global LRU keyed
	// by SQL text. When PrepareStmtMaxSize/PrepareStmtTTL are left at zero, GORM falls
	// back to math.MaxInt entries with a 24h TTL, i.e. effectively unbounded. Because
	// this codebase emits highly variable SQL (dynamic filter/sort/pagination and
	// GORM's IN (...) slice expansion, whose placeholder count changes the query
	// text), the cache grows steadily under normal use. Bounding size and TTL keeps
	// hot queries prepared while evicting the long tail.
	preparedStmtMaxSize = 256
	preparedStmtTTL     = 15 * time.Minute
)

var customGormLogger logger.Interface

func SetGormLogger(l logger.Interface) {
	customGormLogger = l
}

func Initialize(ctx context.Context, databaseURL string, options MigrationOptions) (database *DB, err error) {
	db, err := connectDatabaseInternal(ctx, databaseURL)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to database: %w", err)
	}
	// Initialization opens a connection pool before it can fail on migrations or
	// provider detection; release it rather than leaking it into a failed startup.
	defer func() {
		if err == nil {
			return
		}
		if closeErr := db.Close(); closeErr != nil {
			slog.WarnContext(ctx, "Failed to close database after initialization failure", "error", closeErr)
		}
	}()

	if cancellationErr := ctx.Err(); cancellationErr != nil {
		return nil, cancellationErr
	}

	// Get underlying sql.DB for migrations
	sqlDB, err := db.DB.DB()
	if err != nil {
		return nil, fmt.Errorf("failed to get sql.DB: %w", err)
	}

	if migrateDatabaseErr := migrateDatabaseInternal(ctx, sqlDB, dbProviderPostgres, options); migrateDatabaseErr != nil {
		slog.ErrorContext(ctx, "Failed to run migrations", "error", migrateDatabaseErr)
		return nil, fmt.Errorf("failed to run migrations: %w", migrateDatabaseErr)
	}

	// Set connection pool settings
	sqlDB.SetMaxIdleConns(15)
	sqlDB.SetMaxOpenConns(50)
	sqlDB.SetConnMaxLifetime(5 * time.Minute)
	sqlDB.SetConnMaxIdleTime(3 * time.Minute)

	return db, nil
}

func connectDatabaseInternal(ctx context.Context, databaseURL string) (*DB, error) {
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return nil, errors.New("DATABASE_URL must be a PostgreSQL URL")
	}
	dialector := postgres.Open(databaseURL)

	// Retry connection up to 3 times
	var db *gorm.DB
	var err error
	for i := 1; i <= 3; i++ {
		if retryCancellationErr := ctx.Err(); retryCancellationErr != nil {
			return nil, retryCancellationErr
		}
		db, err = gorm.Open(dialector, &gorm.Config{
			Logger: customGormLogger,
			NowFunc: func() time.Time {
				return time.Now().UTC()
			},
			PrepareStmt:                      true,
			PrepareStmtMaxSize:               preparedStmtMaxSize,
			PrepareStmtTTL:                   preparedStmtTTL,
			IgnoreRelationshipsWhenMigrating: true,
		})
		if err == nil {
			return &DB{db}, nil
		}

		slog.InfoContext(ctx, "Failed to initialize database", "attempt", i)
		if i < 3 {
			select {
			case <-time.After(3 * time.Second):
			case <-ctx.Done():
				return nil, ctx.Err()
			}
		}
	}

	return nil, err
}

func migrateDatabaseInternal(ctx context.Context, db *sql.DB, dbProvider string, options MigrationOptions) error {
	requiredVersion, err := getHighestEmbeddedMigrationVersionInternal(dbProvider)
	if err != nil {
		return fmt.Errorf("failed to determine target migration version for %s: %w", dbProvider, err)
	}

	return migrateDatabaseToVersionInternal(ctx, db, dbProvider, options, requiredVersion)
}

func migrateDatabaseToVersionInternal(ctx context.Context, db *sql.DB, dbProvider string, _ MigrationOptions, requiredVersion int64) error {
	var legacyExists bool
	if err := db.QueryRowContext(ctx, `SELECT to_regclass($1) IS NOT NULL`, legacyVersionTable).Scan(&legacyExists); err != nil {
		return fmt.Errorf("check legacy migration state: %w", err)
	}
	if legacyExists {
		return errors.New("legacy databases are not supported; start with an empty PostgreSQL database")
	}
	provider, err := newGooseProviderInternal(db, dbProvider)
	if err != nil {
		return fmt.Errorf("create goose provider: %w", err)
	}
	currentVersion, err := provider.GetDBVersion(ctx)
	if err != nil {
		return fmt.Errorf("read migration version: %w", err)
	}
	if currentVersion > requiredVersion {
		return fmt.Errorf("database schema version %d is newer than supported version %d", currentVersion, requiredVersion)
	}
	if currentVersion == requiredVersion {
		return nil
	}
	if _, err := provider.UpTo(ctx, requiredVersion); err != nil {
		return fmt.Errorf("apply PostgreSQL migrations: %w", err)
	}
	return nil
}

func newGooseProviderInternal(db *sql.DB, dbProvider string) (*goose.Provider, error) {
	migrationsFS, err := embeddedMigrationFSInternal(dbProvider)
	if err != nil {
		return nil, err
	}

	dialect, err := gooseDialectInternal(dbProvider)
	if err != nil {
		return nil, err
	}

	// Two Arcane processes pointed at the same Postgres (a rolling deploy, or a
	// replica set) would otherwise run migrations concurrently and race on the
	// same DDL. A session-level advisory lock serializes them.
	var options []goose.ProviderOption
	if dialect == goose.DialectPostgres {
		sessionLocker, newPostgresSessionLockerErr := lock.NewPostgresSessionLocker()
		if newPostgresSessionLockerErr != nil {
			return nil, fmt.Errorf("failed to create Postgres migration session locker: %w", newPostgresSessionLockerErr)
		}
		options = append(options, goose.WithSessionLocker(sessionLocker))
	}

	return goose.NewProvider(dialect, db, migrationsFS, options...)
}

func embeddedMigrationFSInternal(dbProvider string) (fs.FS, error) {
	migrationsFS, err := fs.Sub(resources.FS, "migrations/"+dbProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to load embedded migrations for %s: %w", dbProvider, err)
	}

	return migrationsFS, nil
}

func gooseDialectInternal(dbProvider string) (goose.Dialect, error) {
	switch dbProvider {
	case dbProviderPostgres:
		return goose.DialectPostgres, nil
	default:
		return "", fmt.Errorf("unsupported database provider: %s", dbProvider)
	}
}

func getHighestEmbeddedMigrationVersionInternal(dbProvider string) (int64, error) {
	versions, err := getEmbeddedMigrationVersionsInternal(dbProvider)
	if err != nil {
		return 0, err
	}
	if len(versions) == 0 {
		return 0, fmt.Errorf("no embedded migrations found for %s", dbProvider)
	}

	return versions[len(versions)-1], nil
}

func getEmbeddedMigrationVersionsInternal(dbProvider string) ([]int64, error) {
	entries, err := resources.FS.ReadDir("migrations/" + dbProvider)
	if err != nil {
		return nil, fmt.Errorf("failed to read embedded migrations for %s: %w", dbProvider, err)
	}

	versionsMap := make(map[int64]struct{})
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".sql") {
			continue
		}

		versionText, _, found := strings.Cut(entry.Name(), "_")
		if !found {
			continue
		}
		version, parseErr := strconv.ParseInt(versionText, 10, 64)
		if parseErr != nil {
			continue
		}

		versionsMap[version] = struct{}{}
	}

	versions := make([]int64, 0, len(versionsMap))
	for version := range versionsMap {
		versions = append(versions, version)
	}
	slices.Sort(versions)

	return versions, nil
}

// FindEnvironmentIDByApiKey finds the environment ID that is associated with the given API key.
// It queries the api_keys table to validate the key and find the associated environment.
func (db *DB) FindEnvironmentIDByApiKey(ctx context.Context, apiKey string) (string, error) {
	var envID string
	err := db.WithContext(ctx).Table("environments").
		Select("environments.id").
		Joins("INNER JOIN api_keys ON api_keys.id = environments.api_key_id").
		Where("api_keys.key = ?", apiKey).
		Pluck("environments.id", &envID).Error
	if err != nil {
		return "", err
	}
	if envID == "" {
		return "", gorm.ErrRecordNotFound
	}
	return envID, nil
}

func (db *DB) Close() error {
	sqlDB, err := db.SQLDB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

func (db *DB) SQLDB() (*sql.DB, error) {
	return db.DB.DB()
}
