// Source: docklet-next/issues/286
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package backup

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/backuptarget"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	dto "github.com/florianfinn/docklet-next/types/backup"
	"github.com/florianfinn/docklet-next/types/base"
	"net/http"
	"time"
)

func (s *Service) configureDraftProbe(h *backuptarget.Handler, environments *environment.EnvironmentService) {
	h.Probe = func(ctx context.Context, request dto.TargetProbeRequest) (dto.TargetProbeResult, error) {
		input, err := backuptarget.Validate(request.Target)
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		host, err := s.hostID(ctx)
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		if host != request.ProbeHostID || input.HostID != nil && *input.HostID != host {
			return dto.TargetProbeResult{}, dto.NewProblem(422, "backup.target.probe-host-mismatch", nil)
		}
		cli, err := s.deps.Docker.GetClient(ctx)
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		image, err := s.deps.HelperImage(ctx, cli)
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		return backuptarget.Probe(ctx, cli, image, backuptarget.DraftTarget(input), host, s.deps.Exec)
	}
	h.DraftTest = func(ctx context.Context, request dto.TargetDraftTest) (dto.TargetProbeResult, error) {
		ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
		defer cancel()
		input, err := s.targets.Draft(ctx, request)
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		if request.ProbeHostID == "0" {
			return h.Probe(ctx, dto.TargetProbeRequest{Target: input, ProbeHostID: "0"})
		}
		var host struct{ Enabled bool }
		result := s.deps.DB.WithContext(ctx).Table("environments").Select("enabled").Where("id = ?", request.ProbeHostID).Limit(1).Find(&host)
		if result.Error != nil {
			return dto.TargetProbeResult{}, result.Error
		}
		if result.RowsAffected == 0 || !host.Enabled {
			return dto.TargetProbeResult{}, dto.NewProblem(422, "backup.target.wrong-host", nil)
		}
		if environments == nil {
			return dto.TargetProbeResult{}, dto.NewProblem(503, "backup.target.sync-failed", nil)
		}
		out, err := backuptarget.RemoteJSON[base.ApiResponse[dto.TargetProbeResult]](ctx, environments.ProxyJSONRequest, request.ProbeHostID, http.MethodPost, "/api/environments/0/volumes/backups/targets/probe", dto.TargetProbeRequest{Target: input, ProbeHostID: request.ProbeHostID})
		if err != nil {
			return dto.TargetProbeResult{}, err
		}
		return out.Data, nil
	}
}
