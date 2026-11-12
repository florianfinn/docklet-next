// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-04
// License: Apache-2.0
// Deviations: Original hub zone provider; agents retain their process TZ.

package settings

import (
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/scheduler/schedule"
)

// BindZoneProvider is called during dependency construction, before consumers
// start. The settings snapshot is atomic and the fallback is the process TZ.
func (s *SettingsService) BindZoneProvider(cfg *config.Config) {
	if cfg == nil || cfg.ZoneProvider != nil {
		return
	}
	fallback := cfg.GetLocation()
	cfg.ZoneProvider = schedule.ZoneFunc(func() *time.Location {
		if !cfg.AgentMode && !cfg.EdgeAgent {
			value := s.GetSettingsConfig().TimeZone.Value
			if value != "" {
				if loc, err := time.LoadLocation(value); err == nil {
					return loc
				}
			}
		}
		return fallback
	})
}
