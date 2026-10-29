// Source: docklet-next/issues/283
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original last-observed snapshot availability from explicit engine reads.

package backup

import (
	"context"
	"errors"
	"strings"

	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/rustic"
	dto "github.com/florianfinn/docklet-next/types/backup"
)

func observeSnapshotResult(ctx context.Context, repository Repository, id string, err error) error {
	if repository.ObserveSnapshot == nil {
		return err
	}
	missing := false
	var commandError *rustic.CommandError
	if errors.As(err, &commandError) {
		// Full snapshot IDs are direct backend reads in the pinned Rustic producer.
		// Match that file and ENOENT together; a missing node or pack is not a missing snapshot.
		diagnostic := commandError.Diagnostic
		missing = strings.Contains(diagnostic, "/snapshots/"+id) && strings.Contains(diagnostic, "No such file or directory (os error 2)")
	}
	if err != nil && !missing {
		return err
	}
	observationErr := repository.ObserveSnapshot(context.WithoutCancel(ctx), id, !missing)
	if missing {
		return errors.Join(dto.NewProblem(404, "volume.backup.snapshot-not-found", nil), observationErr)
	}
	return observationErr
}
