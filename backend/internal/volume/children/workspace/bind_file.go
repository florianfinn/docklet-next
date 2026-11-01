// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package workspace

import (
	"context"
	"fmt"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/volume"
	"github.com/florianfinn/docklet-next/types/workspace"
	"path"
	"strconv"
)

// File mounts cannot be replaced by rename/unlink. Keep all guards before staging.
func (s *Service) updateBindFile(ctx context.Context, id, key string, manifest volume.WorkspaceUpdateManifest, uploads map[int][]byte) (*workspace.Workspace, error) {
	source, err := volume.BindSource(key)
	if err != nil {
		return nil, err
	}
	basename := path.Base(source)
	if len(manifest.FileChanges) != 1 || manifest.FileChanges[0].Operation != volume.FileOpUpdateFile || manifest.FileChanges[0].RelativePath != basename {
		return nil, dto.NewProblem(409, "volume.bind.file-operation-unsupported", nil)
	}
	change := manifest.FileChanges[0]
	if change.BaselineIndex != nil {
		if err = s.checkVolumeWorkspaceBaselineInternal(ctx, id, basename, uploads[*change.BaselineIndex]); err != nil {
			return nil, err
		}
	}
	cli, err := s.deps.Docker.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	identity := s.resolveVolumeWorkspaceWriteIdentityInternal(ctx, cli, id, key)
	staged, err := s.stageVolumeWorkspaceChangesInternal(ctx, cli, id, manifest.FileChanges, uploads, identity)
	if err != nil {
		return nil, err
	}
	_, _, err = s.deps.Exec(ctx, id, "", []string{"sh", "-c", bindFileWriteScript, "sh", path.Join("/volume", basename), staged[0].path, strconv.FormatInt(staged[0].size, 10)})
	if err != nil {
		if code, _, probeErr := s.deps.Exec(ctx, id, "", []string{"sh", "-c", `if test -f /tmp/arcane-workspace/recovery-failed; then printf failed; fi`}); probeErr != nil || code == "failed" {
			return nil, dto.NewProblem(500, "volume.bind.recovery-failed", err)
		}
		return nil, fmt.Errorf("write bind file: %w", err)
	}
	return s.readVolumeWorkspaceFromContainerInternal(ctx, id)
}

const bindFileWriteScript = bindpath.FileWriteScript
