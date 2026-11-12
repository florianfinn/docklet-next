// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original ordered runtime condition classifier and caller-owned snapshots; M19 backend contracts and target identity integration.

package history

import (
	dto "github.com/florianfinn/docklet-next/types/history"
	"slices"
	"sync"
	"time"
)

// Classify follows Docker state before applying the running-only restart window.
func Classify(c dto.ContainerSample, restarts int) (dto.State, string) {
	switch c.State {
	case "restarting":
		return dto.Unhealthy, "restart-loop"
	case "running":
		if restarts >= 3 {
			return dto.Unhealthy, "restart-loop"
		}
		if c.Health == "unhealthy" {
			return dto.Unhealthy, "health"
		}
		return dto.Running, ""
	case "paused":
		return dto.Stopped, "paused"
	case "created", "exited", "dead":
		if c.StopIntent {
			return dto.Stopped, "intent"
		}
		if c.State == "created" && neverStarted(c.StartedAt) {
			return dto.Stopped, "never-started"
		}
		if c.OOMKilled {
			return dto.Down, "oom"
		}
		if c.State == "dead" {
			return dto.Down, "exit"
		}
		if c.ExitCode == 0 || c.ExitCode == 143 {
			return dto.Stopped, "operator-stop"
		}
		return dto.Down, "exit"
	}
	return dto.NoData, ""
}
func neverStarted(value string) bool {
	t, err := time.Parse(time.RFC3339Nano, value)
	return value == "" || err == nil && t.IsZero()
}

type restartPoint struct {
	Minute int64 `json:"minute"`
	Count  int   `json:"count"`
}

func restartWindow(points []restartPoint, minute int64, count int) ([]restartPoint, int) {
	first := 0
	for first < len(points) && points[first].Minute < minute-9*60 {
		first++
	}
	points = slices.Clone(points[first:])
	if len(points) > 0 && count < points[len(points)-1].Count {
		points = nil
	}
	points = append(points, restartPoint{minute, count})
	return points, count - points[0].Count
}

type snapshots struct {
	mu    sync.RWMutex
	hosts map[string]dto.Conditions
}

func cloneConditions(c dto.Conditions) dto.Conditions {
	c.Items = slices.Clone(c.Items)
	if c.Items == nil {
		c.Items = []dto.Condition{}
	}
	return c
}
func (s *snapshots) publish(id string, c dto.Conditions) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.hosts == nil {
		s.hosts = map[string]dto.Conditions{}
	}
	s.hosts[id] = cloneConditions(c)
}
func (s *snapshots) read(id string) dto.Conditions {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return cloneConditions(s.hosts[id])
}
func (s *snapshots) forget(id string) { s.mu.Lock(); defer s.mu.Unlock(); delete(s.hosts, id) }

func stackState(conditions []dto.Condition, status string) dto.State {
	if len(conditions) == 0 {
		if status == "stopped" {
			return dto.Stopped
		}
		return dto.NoData
	}
	state := dto.Stopped
	for _, c := range conditions {
		switch c.Condition {
		case dto.Down:
			return dto.Down
		case dto.Unhealthy:
			state = dto.Unhealthy
		case dto.Running:
			if state != dto.Unhealthy {
				state = dto.Running
			}
		}
	}
	return state
}
