// Source: docklet-next/docs/design/images-updates.md
// Revision: c2a8b95f3eae4f7d561bfc48d5dfe97c63bab6db
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Explicit metadata reads only; no watcher, image pull or replacement.

package updater

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
	"github.com/distribution/reference"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/environment"
	"github.com/florianfinn/docklet-next/backend/internal/middleware"
	"github.com/florianfinn/docklet-next/backend/internal/version"
	"github.com/florianfinn/docklet-next/backend/pkg/authz"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/handlerutil"
	"github.com/florianfinn/docklet-next/types/base"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	dto "github.com/florianfinn/docklet-next/types/updater"
	versionDTO "github.com/florianfinn/docklet-next/types/version"
	"net/http"
	"strings"
)

func (s *UpdaterService) ConfigureSystemImageHints(envs *environment.EnvironmentService, v *version.VersionService) {
	if s.policies == nil {
		return
	}
	s.policies.ports.hints = func(ctx context.Context, env string) (dto.SystemImageHints, error) {
		var info versionDTO.Info
		if env == "0" {
			if v != nil {
				info = *v.GetAppVersionInfo(ctx)
			}
		} else {
			if err := envs.ProxyJSONRequest(ctx, env, http.MethodGet, "/api/app-version", nil, &info); err != nil {
				return dto.SystemImageHints{}, imageDTO.Failure("update.check-unavailable", 503)
			}
		}
		latest, best := "", ""
		if v != nil {
			latest, _ = v.GetLatestVersion(ctx)
			best, _ = v.PublishedReleaseAtMost(ctx, config.Version)
		}
		configured := strings.TrimSpace(s.config.AgentImage)
		if configured == "" {
			configured = "docklet-next-agent:" + strings.TrimPrefix(config.Version, "v")
		}
		return systemImageHints(config.Version, configured, info, latest, best), nil
	}
}
func hintTag(ref string) (string, string) {
	named, err := reference.ParseNormalizedNamed(ref)
	if err != nil {
		return "", ""
	}
	tagged, ok := named.(reference.Tagged)
	if !ok {
		return "", ""
	}
	if _, valid := imageDTO.CompareRelease(tagged.Tag(), tagged.Tag()); !valid {
		return "", ""
	}
	return reference.TrimNamed(named).String(), strings.TrimPrefix(tagged.Tag(), "v")
}
func systemImageHints(hub, configured string, info versionDTO.Info, latest, best string) dto.SystemImageHints {
	unknown := func(kind, ref string) dto.SystemImageHint {
		return dto.SystemImageHint{Kind: kind, Reference: ref, State: "unknown", Code: "update.system-version-unknown", ManualOnly: true}
	}
	agent := unknown("agent", info.CurrentImageRef)
	tools := unknown("tools", info.ToolsImage)
	repository, currentTag := hintTag(info.CurrentImageRef)
	allowed, _ := hintTag(configured)
	canonical, _ := hintTag("docklet-next-agent:0.0.0")
	if allowed != canonical {
		allowed = ""
	}
	comparison, valid := imageDTO.CompareRelease(info.CurrentVersion, hub)
	if valid && repository != "" && repository == allowed && strings.TrimPrefix(info.CurrentVersion, "v") == currentTag {
		agent.CurrentVersion = currentTag
		if comparison > 0 {
			agent.RequiresHubUpdate = true
			agent.Code = "update.requires-hub-update"
		} else {
			agent.State = "current"
			agent.Code = ""
			if c, ok := imageDTO.CompareRelease(latest, hub); ok && c > 0 {
				agent.RequiresHubUpdate = true
			}
			upper, ok := imageDTO.CompareRelease(best, hub)
			newer, observed := imageDTO.CompareRelease(best, currentTag)
			if ok && observed && upper <= 0 && newer > 0 {
				agent.State = "available"
				agent.CandidateVersion = strings.TrimPrefix(best, "v")
			} else if !ok {
				agent.State = "unknown"
				agent.Code = "update.check-unavailable"
			}
		}
	}
	toolsRepo, toolsVersion := hintTag(info.ToolsImage)
	compatibleRepo, _ := hintTag(volumehelper.DefaultToolsImage)
	if toolsRepo != "" && toolsRepo == compatibleRepo {
		tools.CurrentVersion = toolsVersion
		if info.ToolsImageOverride {
			tools.State = "pinned"
			tools.Code = ""
		} else if valid && comparison == 0 {
			// This binary declares the mapping only for its own release. Older agents'
			// tools compatibility remains unknown instead of borrowing the hub's mapping.
			tools.CompatibleReference = volumehelper.DefaultToolsImage
			if info.ToolsImage == volumehelper.DefaultToolsImage {
				tools.State = "current"
				tools.Code = ""
			}
		}
	}
	return dto.SystemImageHints{HubVersion: hub, Images: []dto.SystemImageHint{agent, tools}}
}
func registerSystemImageHintRoutes(api huma.API, s *UpdaterService) {
	middleware.RegisterWithPermission(api, huma.Operation{OperationID: "get-system-image-hints", Method: http.MethodGet, Path: "/environments/{id}/updater/system-image-hints", Tags: []string{"Updater"}, Summary: "Read manual-only agent/tools hints capped by the running hub"}, authz.PermImageUpdatesRead, func(ctx context.Context, in *struct {
		EnvironmentID string `path:"id"`
	}) (*handlerutil.Out[dto.SystemImageHints], error) {
		p, err := s.hubPolicies()
		if err != nil {
			return nil, err
		}
		if p.ports.hints == nil {
			return nil, imageDTO.Failure("update.check-unavailable", 503)
		}
		result, err := p.ports.hints(ctx, in.EnvironmentID)
		return &handlerutil.Out[dto.SystemImageHints]{Body: base.ApiResponse[dto.SystemImageHints]{Success: err == nil, Data: result}}, err
	})
}
