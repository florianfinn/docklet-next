// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original typed repository initialization and metadata-only verification.

package backup

import (
	"context"
	"github.com/moby/moby/client"
)

// CheckRepository initializes only an operation-witnessed empty directory.
// An existing repository opened with a wrong key never receives an init command.
func (e *Engine) CheckRepository(ctx context.Context, cli *client.Client, repository Repository, password string, initialize bool) (int64, error) {
	if initialize {
		if _, err := e.runInternal(ctx, cli, repository, password, []string{"init"}); err != nil {
			return 0, err
		}
	}
	snapshots, err := e.ListSnapshots(ctx, cli, repository, password)
	if err != nil {
		return 0, err
	}
	if _, err = e.runInternal(ctx, cli, repository, password, []string{"check"}); err != nil {
		return 0, err
	}
	return int64(len(snapshots)), nil
}
