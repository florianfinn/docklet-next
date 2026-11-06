// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original notification and Auto Heal foundation; M19 backend contracts and target identity integration.

package settings

import (
	"context"
	"github.com/florianfinn/docklet-next/backend/internal/config"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	dto "github.com/florianfinn/docklet-next/types/autoheal"
	settingsDTO "github.com/florianfinn/docklet-next/types/settings"
	"strconv"
	"strings"
)

var AutoHealKeys = []string{"autoHealEnabled", "autoHealInterval", "autoHealExcludedContainers", "autoHealExcludedStacks", "autoHealMaxRestarts", "autoHealRestartWindow", "autoHealCooldownMinutes"}

// AutoHealManagedByEnvironment uses the same presence semantics as the effective loader.
func AutoHealManagedByEnvironment() bool {
	cfg := config.LoadRole()
	if !cfg.AgentMode && !cfg.EdgeAgent && !cfg.UIConfigurationDisabled {
		return false
	}
	for _, name := range []string{"AUTO_HEAL_ENABLED", "AUTO_HEAL_INTERVAL", "AUTO_HEAL_EXCLUDED_CONTAINERS", "AUTO_HEAL_EXCLUDED_STACKS", "AUTO_HEAL_MAX_RESTARTS", "AUTO_HEAL_RESTART_WINDOW", "AUTO_HEAL_COOLDOWN_MINUTES"} {
		if _, found, _ := utils.LookupEnvOrFile(name); found {
			return true
		}
	}
	return false
}
func AutoHealUpdateManaged(u settingsDTO.Update) bool {
	return u.AutoHealEnabled != nil || u.AutoHealInterval != nil || u.AutoHealExcludedContainers != nil || u.AutoHealExcludedStacks != nil || u.AutoHealMaxRestarts != nil || u.AutoHealRestartWindow != nil || u.AutoHealCooldownMinutes != nil
}
func names(v string) []string {
	r := []string{}
	for _, n := range strings.Split(v, ",") {
		if n = strings.TrimSpace(n); n != "" {
			r = append(r, n)
		}
	}
	return r
}
func (s *SettingsService) AutoHealSettings(ctx context.Context) (dto.Settings, error) {
	cfg, err := s.GetSettings(ctx)
	if err != nil {
		return dto.Settings{}, err
	}
	result := dto.Settings{Enabled: cfg.AutoHealEnabled.IsTrue(), MaxRestarts: cfg.AutoHealMaxRestarts.AsInt(), WindowMinutes: cfg.AutoHealRestartWindow.AsInt(), CooldownMinutes: cfg.AutoHealCooldownMinutes.AsInt(), ExcludedStacks: names(cfg.AutoHealExcludedStacks.Value), ExcludedContainers: names(cfg.AutoHealExcludedContainers.Value), ManagedByEnvironment: AutoHealManagedByEnvironment()}
	result.InvalidFields = result.InvalidFieldNames()
	return result, nil
}
func (s *SettingsService) UpdateAutoHeal(ctx context.Context, u dto.Update) (dto.Settings, error) {
	if err := u.Validate(); err != nil {
		return dto.Settings{}, err
	}
	if AutoHealManagedByEnvironment() {
		return dto.Settings{}, &dto.Problem{Code: "autoheal.settings-env-managed", Status: 409}
	}
	updates := []libarcane.SettingUpdate{{Key: "autoHealEnabled", Value: strconv.FormatBool(u.Enabled)}, {Key: "autoHealMaxRestarts", Value: strconv.Itoa(u.MaxRestarts)}, {Key: "autoHealRestartWindow", Value: strconv.Itoa(u.WindowMinutes)}, {Key: "autoHealExcludedStacks", Value: strings.Join(u.ExcludedStacks, ",")}, {Key: "autoHealExcludedContainers", Value: strings.Join(u.ExcludedContainers, ",")}}
	if u.CooldownMinutes != nil {
		updates = append(updates, libarcane.SettingUpdate{Key: "autoHealCooldownMinutes", Value: strconv.Itoa(*u.CooldownMinutes)})
	}
	s.writes.Lock()
	defer s.writes.Unlock()
	current := s.GetSettingsConfig()
	values := make([]SettingVariable, 0, len(updates))
	changes := make([]libarcane.SettingUpdate, 0, len(updates))
	for _, update := range updates {
		value, _, _, err := current.FieldByKey(update.Key)
		if err != nil {
			return dto.Settings{}, err
		}
		if value != update.Value {
			changes = append(changes, update)
			values = append(values, SettingVariable{Key: update.Key, Value: update.Value})
		}
	}
	if len(values) > 0 {
		if err := s.persistSettings(ctx, values); err != nil {
			return dto.Settings{}, err
		}
		if err := s.refreshSettingsCacheInternal(context.WithoutCancel(ctx)); err != nil {
			return dto.Settings{}, err
		}
		s.publishSettingsChangesInternal(changes)
	}
	return s.AutoHealSettings(ctx)
}
