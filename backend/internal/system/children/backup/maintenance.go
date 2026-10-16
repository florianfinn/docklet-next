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
	operation  string
	active     int
	drained    chan struct{}
	cancellers map[uint64]func()
	nextID     uint64
	onBegin    []func()
}

func NewMaintenanceGate() *MaintenanceGate { return &MaintenanceGate{} }

// Admit holds the barrier across a request so restore can wait for prior work.
func (g *MaintenanceGate) Admit() (func(), error) {
	return g.admit(nil)
}

// AdmitCancel keeps an HTTP or gRPC request in the drain until its handler exits.
func (g *MaintenanceGate) AdmitCancel(cancel func()) (func(), error) {
	return g.admit(cancel)
}

// AdmitConnection tracks an upgraded connection after its HTTP handler exits.
func (g *MaintenanceGate) AdmitConnection(closeConnection func()) (func(), error) {
	return g.admit(closeConnection)
}

func (g *MaintenanceGate) admit(cancel func()) (func(), error) {
	if g == nil {
		return func() {}, nil
	}
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.phase != "" {
		return nil, ErrMaintenanceActive
	}
	g.active++
	g.nextID++
	id := g.nextID
	if cancel != nil {
		if g.cancellers == nil {
			g.cancellers = make(map[uint64]func())
		}
		g.cancellers[id] = cancel
	}
	return sync.OnceFunc(func() {
		g.mu.Lock()
		defer g.mu.Unlock()
		delete(g.cancellers, id)
		g.active--
		if g.active == 0 && g.drained != nil {
			close(g.drained)
			g.drained = nil
		}
	}), nil
}

func (g *MaintenanceGate) OnBegin(closeConnections func()) {
	g.mu.Lock()
	g.onBegin = append(g.onBegin, closeConnections)
	g.mu.Unlock()
}

func (g *MaintenanceGate) Begin(activityID string, operations ...string) error {
	g.mu.Lock()
	if g.phase != "" {
		g.mu.Unlock()
		return ErrMaintenanceActive
	}
	g.phase, g.activityID = "draining", activityID
	g.operation = "restore"
	if len(operations) > 0 {
		g.operation = operations[0]
	}
	if g.active > 0 {
		g.drained = make(chan struct{})
	}
	cancels := make([]func(), 0, len(g.cancellers)+len(g.onBegin))
	for _, cancel := range g.cancellers {
		cancels = append(cancels, cancel)
	}
	cancels = append(cancels, g.onBegin...)
	g.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
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
	return backuptypes.SystemBackupMaintenance{Active: true, Operation: g.operation, Phase: g.phase, ReasonCode: "backup.maintenance", ActivityID: g.activityID}
}

func (s *Service) MaintenanceGate() *MaintenanceGate {
	if s == nil {
		return nil
	}
	return s.maintenance
}
