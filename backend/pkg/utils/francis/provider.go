// Source: getarcaneapp/arcane/backend/pkg/utils/francis/provider.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: SQLite sidecar with caller-owned single-connection persistence.

package francis

import (
	"context"
	"errors"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/sqlitestore"
	"log/slog"
	"time"

	"database/sql"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/components/sqlite"
)

func StoreURL(databaseURL string) (string, error) {
	path, err := sqlitestore.Path(databaseURL)
	if err != nil {
		return "", err
	}
	return sqlitestore.URL(sqlitestore.FrancisPath(path)), nil
}

type ownedProvider struct {
	components.ActorProvider
	pool *sql.DB
}

func (p *ownedProvider) Close() error { return errors.Join(p.ActorProvider.Close(), p.pool.Close()) }

func openProvider(ctx context.Context, storeURL string) (components.ActorProvider, error) {
	cfg := components.NewProviderConfig()
	cfg.HostHealthCheckDeadline = 90 * time.Second
	cfg.MaxHosts = 1
	path, err := sqlitestore.Path(storeURL)
	if err != nil {
		return nil, err
	}
	pool, err := sqlitestore.Open(ctx, path, false, false)
	if err != nil {
		return nil, err
	}
	provider, err := sqlite.NewSQLiteProvider(slog.Default(), sqlite.SQLiteProviderOptions{DB: pool, TablePrefix: TablePrefix}, cfg)
	if err != nil {
		_ = pool.Close()
		return nil, err
	}
	if err = provider.Init(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("initialize actor provider: %w", err), provider.Close(), pool.Close())
	}
	return &ownedProvider{ActorProvider: provider, pool: pool}, nil
}

// ClearRestoredHosts removes copied live host ownership from an offline restored database.
func ClearRestoredHosts(ctx context.Context, databaseURL string) (err error) {
	storeURL, err := StoreURL(databaseURL)
	if err != nil {
		return err
	}
	provider, err := openProvider(ctx, storeURL)
	if err != nil {
		return fmt.Errorf("open restored actor storage: %w", err)
	}
	defer func() {
		if closeErr := provider.Close(); closeErr != nil {
			err = errors.Join(err, fmt.Errorf("close restored actor provider: %w", closeErr))
		}
	}()
	hosts, err := provider.ListHosts(ctx)
	if err != nil {
		return fmt.Errorf("list restored actor hosts: %w", err)
	}
	for _, host := range hosts {
		err = provider.UnregisterHost(ctx, host.HostID, components.UnregisterHostOpts{})
		if err != nil && !errors.Is(err, components.ErrHostUnregistered) {
			return fmt.Errorf("remove restored actor host: %w", err)
		}
	}
	return nil
}
