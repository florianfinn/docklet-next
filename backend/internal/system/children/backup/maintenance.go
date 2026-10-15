// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original in-process restore admission barrier for issue #32.

package backup

import (
	"context"
	"errors"
	"sync"

	backuptypes "github.com/florianfinn/docklet-next/types/backup"
)

var ErrMaintenanceActive = errors.New("backup.maintenance")

type MaintenanceGate struct {
	mu         sync.Mutex
	phase      string
	activityID string
	active     int
	drained    chan struct{}
}

func NewMaintenanceGate() *MaintenanceGate { return &MaintenanceGate{} }

// Admit holds the barrier across a request so restore can wait for prior work.
func (g *MaintenanceGate) Admit() (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "" {
		return nil, ErrMaintenanceActive
	}
	g.active++
	return func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		g.active--
		if g.active == 0 && g.drained != nil {
			close(g.drained)
			g.drained = nil
		}
	}, nil
}

func (g *MaintenanceGate) Begin(activityID string) error {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "" {
		return ErrMaintenanceActive
	}
	g.phase, g.activityID = "draining", activityID
	if g.active > 0 {
		g.drained = make(chan struct{})
	}
	return nil
}

func (g *MaintenanceGate) Wait(ctx context.Context) error {
	g.mu.Lock()
	ch := g.drained
	g.mu.Unlock()
	if ch == nil {
		return nil
	}
	select {
	case <-ch:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (g *MaintenanceGate) SetPhase(phase string) {
	g.mu.Lock()
	g.phase = phase
	g.mu.Unlock()
}

func (g *MaintenanceGate) End() {
	g.mu.Lock()
	g.phase, g.activityID = "", ""
	g.mu.Unlock()
}

func (g *MaintenanceGate) Status() backuptypes.SystemBackupMaintenance {
	if g == nil {
		return backuptypes.SystemBackupMaintenance{}
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase == "" {
		return backuptypes.SystemBackupMaintenance{}
	}
	return backuptypes.SystemBackupMaintenance{Active: true, ReasonCode: "backup.maintenance", ActivityID: g.activityID}
}

func (s *Service) MaintenanceGate() *MaintenanceGate {
	if s == nil {
		return nil
	}
	return s.maintenance
}
