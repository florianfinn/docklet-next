// Source: docklet-next/docs/design/images-updates.md
// Revision: 361c54cb8d087b7d531ff8bb4dd1acae82f5f534
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original PR1 image and registry safety implementation for issue #229.

package di

import (
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/internal/image"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/internal/system"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane/volumehelper"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/runs"
	imageDTO "github.com/florianfinn/docklet-next/types/image"
	"strings"
)

func wireImageSafety(images *image.ImageService, sys *system.SystemService, gate *runs.Admission, cfg *config.Config, settings *settings.SettingsService) {
	refs := func() []string {
		agent := strings.TrimSpace(cfg.AgentImage)
		if agent == "" {
			tag := strings.TrimPrefix(config.Version, "v")
			if _, ok := imageDTO.CompareRelease(tag, tag); !ok {
				tag = "0.0.0"
			}
			agent = "docklet-next-agent:" + tag
		}
		return []string{agent, volumehelper.ToolsImage(settings.GetSettingsConfig().ToolsImage.Value)}
	}
	images.SetImageAdmission(gate, refs)
	sys.SetImageGuard(images.ImageGuard())
}
