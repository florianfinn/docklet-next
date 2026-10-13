// Source: getarcaneapp/arcane/backend/pkg/utils/francis/provider.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: functional cut: removed SQLite actor storage; functional cut: removed SQLite runtime and migrations.

package francis

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"time"

	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/components/postgres"
	"github.com/italypaleale/francis/host/local"
)

func StoreURL(databaseURL string) (string, error) {
	if !strings.HasPrefix(databaseURL, "postgres://") && !strings.HasPrefix(databaseURL, "postgresql://") {
		return "", errors.New("actor storage requires a PostgreSQL URL")
	}
	return databaseURL, nil
}

func providerOption(storeURL string) local.HostOption {
	return local.WithPostgresProvider(postgres.PostgresProviderOptions{ConnectionString: storeURL, TablePrefix: TablePrefix})
}

func openProvider(ctx context.Context, storeURL string) (components.ActorProvider, error) {
	cfg := components.NewProviderConfig()
	cfg.HostHealthCheckDeadline = 90 * time.Second
	cfg.MaxHosts = 1
	provider, err := postgres.NewPostgresProvider(slog.Default(), postgres.PostgresProviderOptions{ConnectionString: storeURL, TablePrefix: TablePrefix}, cfg)
	if err != nil {
		return nil, err
	}
	if err = provider.Init(ctx); err != nil {
		return nil, errors.Join(fmt.Errorf("initialize actor provider: %w", err), provider.Close())
	}
	return provider, nil
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
