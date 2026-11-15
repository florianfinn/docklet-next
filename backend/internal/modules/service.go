// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original optional-module admission and retained-data lifecycle.

// Package modules owns hub-wide optional-module admission, never host configuration files.
package modules

import (
	"context"
	"fmt"
	"sync"

	"github.com/florianfinn/docklet-next/backend/internal/settings"
	"github.com/florianfinn/docklet-next/backend/pkg/libarcane"
	"github.com/florianfinn/docklet-next/types/features"
	dto "github.com/florianfinn/docklet-next/types/modules"
)

// Hooks read persisted unfinished records and delete only the module's own data.
// Register during startup. Recovery must settle unfinished records before disabling.
type Hooks struct {
	Unfinished func(context.Context) (string, error)
	DeleteData func(context.Context) error
}

type gate struct {
	sync.Mutex
	hooks Hooks
}
type Service struct {
	settings *settings.SettingsService
	gates    map[features.ID]*gate
	agent    bool
}

// SetAgentMode is startup-only. Agents follow the hub's synchronised switch-off;
// their own unfinished journal never vetoes it, because the hub gate decided first.
func (s *Service) SetAgentMode(agent bool) { s.agent = agent }

func New(s *settings.SettingsService) *Service {
	m := &Service{settings: s, gates: map[features.ID]*gate{}}
	for _, d := range features.Modules() {
		m.gates[d.ID] = &gate{}
	}
	if s != nil {
		s.SetModuleWriteGuard(m.GuardSettings)
	}
	return m
}

func (s *Service) Register(id features.ID, hooks Hooks) error {
	g, ok := s.gates[id]
	if !ok {
		return fmt.Errorf("unknown module %q", id)
	}
	g.Lock()
	defer g.Unlock()
	g.hooks = hooks
	return nil
}

func (s *Service) Enabled(ctx context.Context, id features.ID) bool {
	return s != nil && s.settings.IsFeatureEnabled(ctx, id)
}

// Admit commits the pending record and reservation while holding the same gate as
// settings writes. fn must not write settings or retain the gate during execution.
func (s *Service) Admit(ctx context.Context, id features.ID, fn func(context.Context) error) error {
	g, ok := s.gates[id]
	if !ok {
		return fmt.Errorf("unknown module %q", id)
	}
	g.Lock()
	defer g.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if !s.Enabled(ctx, id) {
		return &dto.Problem{Status: 403, Code: "feature-disabled", Feature: id}
	}
	return fn(ctx)
}

func unfinished(ctx context.Context, id features.ID, g *gate) error {
	if g.hooks.Unfinished == nil {
		return nil
	}
	operation, err := g.hooks.Unfinished(ctx)
	if err != nil {
		return err
	}
	if operation != "" {
		return &dto.Problem{Status: 409, Code: "module-busy", Feature: id, Operation: operation}
	}
	return nil
}

// GuardSettings holds gates in registry order until the settings commit and cache
// publication finish. No in-memory reservation substitutes for persisted records.
func (s *Service) GuardSettings(ctx context.Context, updates []libarcane.SettingUpdate) (func(), error) {
	var locked []*gate
	release := func() {
		for i := len(locked) - 1; i >= 0; i-- {
			locked[i].Unlock()
		}
	}
	for _, d := range features.Modules() {
		for _, u := range updates {
			if u.Key != d.SettingKey {
				continue
			}
			g := s.gates[d.ID]
			g.Lock()
			locked = append(locked, g)
			if err := ctx.Err(); err != nil {
				release()
				return nil, err
			}
			if u.Value != "true" && !s.agent {
				if err := unfinished(ctx, d.ID, g); err != nil {
					release()
					return nil, err
				}
			}
			break
		}
	}
	return release, nil
}

// DeleteData keeps admission closed through deletion. Remote cleanup may follow
// after this interval; it never accesses or removes host configuration files.
func (s *Service) DeleteData(ctx context.Context, id features.ID) error {
	return s.deleteData(ctx, id, nil)
}

func (s *Service) deleteData(ctx context.Context, id features.ID, after func() error) error {
	g, ok := s.gates[id]
	if !ok {
		return fmt.Errorf("unknown module %q", id)
	}
	g.Lock()
	defer g.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	if s.Enabled(ctx, id) {
		return &dto.Problem{Status: 409, Code: "module-on", Feature: id}
	}
	if err := unfinished(ctx, id, g); err != nil {
		return err
	}
	if g.hooks.DeleteData != nil {
		if err := g.hooks.DeleteData(ctx); err != nil {
			return err
		}
	}
	if after != nil {
		return after()
	}
	return nil
}

// Module jobs implement scheduler.ConditionalJob and are rescheduled on their
// registry setting key, following the vulnerability jobs' settings subscription.
