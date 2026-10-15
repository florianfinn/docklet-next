// Source: getarcaneapp/arcane/backend/internal/settings/service.go
// Revision: 095cb778daba1034e2a39961199e712a75d4ba29
// Retrieved: 2026-10-13
// License: BSD-3-Clause
// Deviations: Local module namespace; functional cut: removed APNS mobile push, template catalogue, GitOps, Swarm, standalone build workshop, OIDC and passkeys.

package settings

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"uuid"

	"github.com/florianfinn/docklet-next/types/category"
	"github.com/florianfinn/docklet-next/types/features"
	searchtypes "github.com/florianfinn/docklet-next/types/search"
	"github.com/samber/mo"
	"go.getarcane.app/kit/pkg"

	"github.com/florianfinn/docklet-next/backend/internal/common"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/search"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/backend/pkg/utils"
	"github.com/florianfinn/docklet-next/backend/pkg/utils/concurrency"
)

const (
	browserSessionSigningKeySize = 64
)

type SettingsService struct {
	db     *database.DB
	config atomic.Pointer[Settings]
	// effectiveConfig is the env-override-applied snapshot, rebuilt whenever
	// config is stored. Overrides are resolved once at startup and never
	// change, so materializing them here turns every read (GetSettings and
	// the typed getters — ~100 call sites, some in per-project loops) into a
	// pointer load instead of a full struct Clone plus a reflection pass.
	// The snapshot is shared: readers must never mutate it.
	effectiveConfig atomic.Pointer[Settings]
	envOverrides    []settingsEnvOverride
	writes          sync.Mutex
	effectsMu       sync.Mutex
	effects         []func()
	effectsWake     chan struct{}
	effectsDone     chan struct{}
	effectsClosed   bool
	lifecycleCtx    context.Context
	changes         *concurrency.Signal[[]libarcane.SettingUpdate]
}

type settingsUpdateResultInternal struct {
	settings []SettingVariable
	changes  []libarcane.SettingUpdate
}

type settingsEnvOverride struct {
	fieldIndex int
	key        string
	envVarName string
	value      string
}

func NewSettingsService(ctx context.Context, db *database.DB) (*SettingsService, error) {
	if ctx == nil {
		return nil, errors.New("settings lifecycle context unavailable")
	}
	svc := &SettingsService{
		db:           db,
		effectsWake:  make(chan struct{}, 1),
		effectsDone:  make(chan struct{}),
		lifecycleCtx: ctx,
		changes:      concurrency.NewSignal[[]libarcane.SettingUpdate](),
	}
	svc.envOverrides = resolveSettingsEnvOverridesInternal()
	if len(svc.envOverrides) > 0 {
		slog.InfoContext(ctx, "Loaded Environment Settings Overrides", "count", len(svc.envOverrides))
	}

	err := svc.LoadDatabaseSettings(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to load settings: %w", err)
	}

	err = svc.setupInstanceID(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to setup instance ID: %w", err)
	}

	go svc.runEffectsInternal()
	return svc, nil
}

// SubscribeSettingsChanges invokes callback once when any subscribed key is
// present in a successful update. The callback receives only matching values.
// Callbacks run serially on the ordered effects worker, outside the write mutex.
func (s *SettingsService) SubscribeSettingsChanges(keys []string, callback func([]libarcane.SettingUpdate)) func() {
	if callback == nil || len(keys) == 0 {
		return func() {}
	}
	subscribed := make(map[string]struct{}, len(keys))
	for _, key := range keys {
		subscribed[key] = struct{}{}
	}
	return s.changes.Subscribe(func(updates []libarcane.SettingUpdate) {
		matching := make([]libarcane.SettingUpdate, 0, len(updates))
		for _, update := range updates {
			if _, ok := subscribed[update.Key]; ok {
				matching = append(matching, update)
			}
		}
		if len(matching) > 0 {
			if err := s.enqueueEffectInternal(func() { callback(matching) }); err != nil && s.lifecycleCtx.Err() == nil {
				slog.ErrorContext(s.lifecycleCtx, "Failed to queue settings change effects", "error", err)
			}
		}
	})
}

// NotifySettingsChanges publishes current values for keys through the settings
// write mutex. It is used when another settings-table workflow performs persistence.
func (s *SettingsService) NotifySettingsChanges(ctx context.Context, keys ...string) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	updates := make([]libarcane.SettingUpdate, 0, len(keys))
	cfg := s.GetSettingsConfig()
	for _, key := range keys {
		value, _, _, err := cfg.FieldByKey(key)
		if err != nil {
			return fmt.Errorf("load changed setting '%s': %w", key, err)
		}
		updates = append(updates, libarcane.SettingUpdate{Key: key, Value: value})
	}
	s.publishSettingsChangesInternal(updates)
	return nil
}

func (s *SettingsService) GetSettingsConfig() *Settings {
	v := s.config.Load()
	if v == nil {
		panic("GetSettingsConfig called before Settings has been loaded")
	}

	return v
}

func (s *SettingsService) LoadDatabaseSettings(ctx context.Context) (err error) {
	dst, err := s.loadDatabaseSettingsInternal(ctx, s.db)
	if err != nil {
		return err
	}

	s.config.Store(dst)

	effective := dst.Clone()
	s.applyEnvOverrides(ctx, effective)
	s.effectiveConfig.Store(effective)

	return nil
}

func (s *SettingsService) refreshSettingsCacheInternal(ctx context.Context) error {
	if err := s.LoadDatabaseSettings(ctx); err != nil {
		return fmt.Errorf("failed to refresh settings cache: %w", err)
	}

	return nil
}

func (s *SettingsService) RefreshAfterRestore(ctx context.Context) error {
	return s.refreshSettingsCacheInternal(ctx)
}

func (s *SettingsService) getDefaultSettings() *Settings {
	return DefaultSettingsConfig()
}

// GetSettings returns the effective (env-override-applied) settings snapshot.
// The snapshot is shared across callers and must be treated as read-only;
// mutation flows go through UpdateSettings, which works on an explicit clone.
func (s *SettingsService) GetSettings(ctx context.Context) (*Settings, error) {
	settingsCfg := s.getEffectiveSettingsConfigInternal(ctx)
	return settingsCfg, nil
}

// GetSettingsOrDefaults is a convenience for hot paths that need a snapshot but cannot
// meaningfully recover from a settings load failure. It logs any error and guarantees a
// non-nil *Settings (defaults: a zero-valued struct, which the SettingVariable helpers
// like kit.ParseOrDefault treat as "use the caller's default").
func (s *SettingsService) GetSettingsOrDefaults(ctx context.Context) *Settings {
	cfg, err := s.GetSettings(ctx)
	if err != nil {
		slog.WarnContext(ctx, "failed to load settings, falling back to defaults", "error", err)
	}
	return kit.Ternary(cfg == nil, &Settings{}, cfg)
}

func (s *SettingsService) getEffectiveSettingsConfigInternal(ctx context.Context) *Settings {
	if effective := s.effectiveConfig.Load(); effective != nil {
		return effective
	}

	// Only reachable before LoadDatabaseSettings has completed in the
	// constructor; fall back to building the snapshot per call.
	settingsCfg := s.GetSettingsConfig().Clone()
	s.applyEnvOverrides(ctx, settingsCfg)
	return settingsCfg
}
func (s *SettingsService) ListSettings(visibility SettingVisibility) []SettingVariable {
	return s.GetSettingsConfig().ToSettingVariableSlice(visibility, true)
}

// GetSettingType returns the type from the setting metadata
func (s *SettingsService) GetSettingType(key string) string {
	rt := reflect.TypeFor[Settings]()
	for field := range rt.Fields() {
		keyTag := field.Tag.Get("key")
		fieldKey, _, _ := strings.Cut(keyTag, ",")
		if fieldKey == key {
			metaTag := field.Tag.Get("meta")
			parts := strings.SplitSeq(metaTag, ";")
			for part := range parts {
				if after, ok := strings.CutPrefix(part, "type="); ok {
					return after
				}
			}
			return "text" // default type
		}
	}
	return "text" // default if not found
}

func (s *SettingsService) setupInstanceID(ctx context.Context) error {
	instanceID := s.GetSettingsConfig().InstanceID.Value
	if instanceID != "" {
		return nil
	}

	err := s.UpdateSetting(ctx, "instanceId", uuid.New().String())
	if err != nil {
		return fmt.Errorf("failed to set instance ID in database: %w", err)
	}

	return nil
}

func (s *SettingsService) settingValue[T any](ctx context.Context, key string, parse func(string) (T, error)) mo.Option[T] {
	cfg := s.getEffectiveSettingsConfigInternal(ctx)
	value, _, _, err := cfg.FieldByKey(key)
	if err != nil || value == "" {
		return mo.None[T]()
	}

	parsed, err := parse(value)
	if err != nil {
		return mo.None[T]()
	}
	return mo.Some(parsed)
}

func (s *SettingsService) GetBoolSetting(ctx context.Context, key string, defaultValue bool) bool {
	return s.settingValue(ctx, key, strconv.ParseBool).OrElse(defaultValue)
}

func (s *SettingsService) GetIntSetting(ctx context.Context, key string, defaultValue int) int {
	return s.settingValue(ctx, key, strconv.Atoi).OrElse(defaultValue)
}

func (s *SettingsService) GetStringSetting(ctx context.Context, key, defaultValue string) string {
	return s.settingValue(ctx, key, func(value string) (string, error) { return value, nil }).OrElse(defaultValue)
}

func (s *SettingsService) SetBoolSetting(ctx context.Context, key string, value bool) error {
	return s.UpdateSetting(ctx, key, strconv.FormatBool(value))
}

func (s *SettingsService) SetIntSetting(ctx context.Context, key string, value int) error {
	return s.UpdateSetting(ctx, key, strconv.Itoa(value))
}

func (s *SettingsService) SetStringSetting(ctx context.Context, key, value string) error {
	return s.UpdateSetting(ctx, key, value)
}

// SetContainerAutoUpdateExclusionInternal adds or removes a container name from
// the autoUpdateExcludedContainers setting. When excluded is true the container
// is added to the list; when false it is removed.
func (s *SettingsService) SetContainerAutoUpdateExclusionInternal(ctx context.Context, containerName string, excluded bool) error {
	s.writes.Lock()
	defer s.writes.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	ordered := kit.Unique(kit.TrimNonEmpty(strings.Split(s.GetStringSetting(ctx, "autoUpdateExcludedContainers", ""), ",")))

	if excluded {
		ordered = kit.Unique(append(ordered, containerName))
	} else {
		filtered := ordered[:0]
		for _, name := range ordered {
			if name != containerName {
				filtered = append(filtered, name)
			}
		}
		ordered = filtered
	}

	if err := s.updateSettingValueNoRefreshInternal(ctx, "autoUpdateExcludedContainers", strings.Join(ordered, ",")); err != nil {
		return err
	}
	return s.refreshSettingsCacheInternal(context.WithoutCancel(ctx))
}
func (s *SettingsService) enqueueEffectInternal(effect func()) error {
	s.effectsMu.Lock()
	defer s.effectsMu.Unlock()
	if s.effectsClosed {
		return errors.New("settings effects stopped")
	}
	s.effects = append(s.effects, effect)
	select {
	case s.effectsWake <- struct{}{}:
	default:
	}
	return nil
}

func (s *SettingsService) runEffectsInternal() {
	defer close(s.effectsDone)
	for {
		s.effectsMu.Lock()
		if len(s.effects) > 0 {
			effect := s.effects[0]
			s.effects[0] = nil
			s.effects = s.effects[1:]
			s.effectsMu.Unlock()
			func() { var err error; defer utils.RecoverToError(&err, "settings effect"); effect() }()
			continue
		}
		closed := s.effectsClosed
		s.effectsMu.Unlock()
		if closed {
			return
		}
		select {
		case <-s.effectsWake:
		case <-s.lifecycleCtx.Done():
			s.effectsMu.Lock()
			s.effectsClosed = true
			s.effectsMu.Unlock()
		}
	}
}

// Stop drains accepted effects and joins the settings worker.
func (s *SettingsService) Stop(ctx context.Context) error {
	s.writes.Lock()
	s.effectsMu.Lock()
	s.effectsClosed = true
	s.effectsMu.Unlock()
	s.writes.Unlock()
	select {
	case s.effectsWake <- struct{}{}:
	default:
	}
	select {
	case <-s.effectsDone:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// IsFeatureEnabled resolves feature availability on this environment.
func (s *SettingsService) IsFeatureEnabled(ctx context.Context, id features.ID) bool {
	definition, ok := features.Lookup(id)
	if !ok {
		return false
	}
	if s == nil {
		return definition.DefaultEnabled
	}
	cfg := s.GetSettingsOrDefaults(ctx)
	value, _, _, err := cfg.FieldByKey(definition.SettingKey)
	if err != nil || value == "" {
		return definition.DefaultEnabled
	}
	enabled, err := strconv.ParseBool(value)
	return kit.Ternary(err != nil, definition.DefaultEnabled, enabled)
}

// RequireFeature rejects operations when their runtime feature is disabled.
func (s *SettingsService) RequireFeature(ctx context.Context, id features.ID) error {
	if s.IsFeatureEnabled(ctx, id) {
		return nil
	}
	return fmt.Errorf("feature %s is disabled: %w", id, common.ErrFeatureDisabled)
}

type SettingsSearchService struct {
	categories []category.Category
}

func NewSettingsSearchService() *SettingsSearchService {
	return &SettingsSearchService{categories: search.BuildCategories[Settings](searchtypes.SettingsProfile)}
}

// GetSettingsCategories returns the category index initialized by the service.
func (s *SettingsSearchService) GetSettingsCategories() []category.Category {
	return s.categories
}
