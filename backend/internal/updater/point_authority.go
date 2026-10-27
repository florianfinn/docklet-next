// Source: docklet-next/docs/design/images-updates.md
// Revision: beec37b5b4339a07a946bb56d19fb8062ba8bc66
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Shared hub restore-point authority check for #229.

package updater

import (
	"context"
	"net/http"
	"net/url"

	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
)

// CheckRestorePointAccess is used only for non-admin callers. Read current
// executor evidence: a reused project ID can still have orphaned old points.
func (s *UpdaterService) CheckRestorePointAccess(ctx context.Context, env, target, selected string, remote imagePolicyProxy) error {
	if target == "" {
		return imageDTO.Failure("update.invalid-target", 400)
	}
	var points []dto.RestorePoint
	if env == "0" {
		if s == nil || s.deps.Projects == nil {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		var err error
		points, err = s.deps.Projects.ListImageRestorePoints(ctx, target, true)
		if err != nil {
			return err
		}
	} else {
		if remote == nil {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		var out base.ApiResponse[[]dto.RestorePoint]
		if err := remote.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/environments/0/updater/project/"+url.PathEscape(target)+"/restore-points", nil, &out); err != nil {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		if !out.Success || out.Data == nil || len(out.Data) > dto.MaxRestorePointResponseCount {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		points = out.Data
	}
	seen := map[string]bool{}
	for _, point := range points {
		if point.TargetID != target || seen[point.ID] {
			return imageDTO.Failure("update.check-unavailable", 503)
		}
		seen[point.ID] = true
		if point.Orphaned && (selected == "" || selected == point.ID) {
			return imageDTO.Failure("update.invalid-target", 400)
		}
	}
	if selected != "" && !seen[selected] {
		return imageDTO.Failure("restore.not-found", 404)
	}
	return nil
}
