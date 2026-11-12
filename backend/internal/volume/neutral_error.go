// Source: docklet-next/issues/227
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"errors"
	"github.com/danielgtaylor/huma/v2"
	"github.com/florianfinn/docklet-next/backend/internal/common"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"log/slog"
)

func neutralVolumeOperationError(err error) error {
	if err == nil {
		return nil
	}
	var status huma.StatusError
	var problem *dto.Problem
	var exists *volume.ProjectVolumeRenameConflictError
	var inUse *volume.ProjectVolumeRenameInUseError
	var space *volume.ProjectVolumeRenameInsufficientSpaceError
	if errors.As(err, &problem) && dto.IsProblemCode(problem.Code) || errors.As(err, &status) && status.GetStatus() < 500 || errors.As(err, &exists) || errors.As(err, &inUse) || errors.As(err, &space) || errors.Is(err, common.ErrBadRequest) || errors.Is(err, common.ErrNotFound) {
		return err
	}
	slog.Error("Volume operation failed", "error", err)
	return dto.NewProblem(500, "volume.operation-failed", err)
}
