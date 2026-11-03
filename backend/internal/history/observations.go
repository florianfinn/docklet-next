// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original host/container counter deltas and project aggregation.

package history

import (
	dto "github.com/florianfinn/docklet-next/types/history"
	"slices"
	"time"
)

func cloneSample(sample dto.Sample) *dto.Sample {
	sample.Containers = slices.Clone(sample.Containers)
	sample.Projects = slices.Clone(sample.Projects)
	for i := range sample.Containers {
		if sample.Containers[i].Stats != nil {
			sample.Containers[i].Stats = new(*sample.Containers[i].Stats)
		}
	}
	return &sample
}

func delta(current, previous float64) *float64 {
	if current < previous {
		return nil
	}
	return new(current - previous)
}
func deltaValue(current, previous float64) float64 {
	d := delta(current, previous)
	if d == nil {
		return 0
	}
	return *d
}
func cpuPercent(current, previous, total, oldTotal float64) *float64 {
	d, t := delta(current, previous), delta(total, oldTotal)
	if d == nil || t == nil || *t <= 0 {
		return nil
	}
	return new(min(100, 100**d / *t))
}
func (s *Service) observations(host string, now time.Time, sample dto.Sample, old hostState, next *hostState) ([]observation, []dto.Condition) {
	h := sample.Host
	hostObservation := observation{kind: "host", key: host, state: dto.Running, memory: new(h.MemoryUsed), memoryLimit: h.MemoryTotal, diskUsed: h.DiskUsed, diskTotal: h.DiskTotal, diskSource: h.DiskSource}
	previous := map[string]dto.ContainerSample{}
	if old.previous != nil {
		prev := old.previous.Host
		hostObservation.cpu = cpuPercent(h.CPUBusy, prev.CPUBusy, h.CPUTotal, prev.CPUTotal)
		hostObservation.rx = deltaValue(h.NetworkRx, prev.NetworkRx)
		hostObservation.tx = deltaValue(h.NetworkTx, prev.NetworkTx)
		for _, c := range old.previous.Containers {
			previous[c.Name] = c
		}
	}
	out := []observation{hostObservation}
	conditions := []dto.Condition{}
	previousConditions := s.snapshots.read(host)
	priorByID := map[string]dto.Condition{}
	for _, c := range previousConditions.Items {
		priorByID[c.Kind+":"+c.ContainerID+":"+c.ProjectID] = c
	}
	stackConditions := map[string][]dto.Condition{}
	stackMetrics := map[string]observation{}
	for _, c := range sample.Containers {
		prior := previous[c.Name]
		points := old.restarts[c.ID]
		points, restarts := restartWindow(points, now.Unix(), c.RestartCount)
		next.restarts[c.ID] = points
		state, reason := Classify(c, restarts)
		condition := dto.Condition{Kind: "container", ContainerID: c.ID, Name: c.Name, ProjectID: c.ProjectID, Condition: state, Reason: reason, ExitCode: c.ExitCode, Restarts10m: restarts, Since: now, SampledAt: now}
		if p, ok := priorByID["container:"+c.ID+":"+c.ProjectID]; ok && p.Condition == state && p.Reason == reason {
			condition.Since = p.Since
		}
		conditions = append(conditions, condition)
		o := observation{kind: "container", key: c.Name, identity: c.ID, state: state, exitCode: c.ExitCode}
		if c.State == "running" && !c.StatsMissing && c.Stats != nil {
			stats := c.Stats
			o.memory = new(stats.MemoryUsage)
			o.memoryLimit = stats.MemoryLimit
			if o.memoryLimit <= 0 || o.memoryLimit > h.MemoryTotal {
				o.memoryLimit = h.MemoryTotal
			}
			if prior.ID == c.ID && prior.Stats != nil && !prior.StatsMissing {
				before := prior.Stats
				o.cpu = cpuPercent(stats.CPUTotal, before.CPUTotal, stats.CPUSystem, before.CPUSystem)
				o.rx = deltaValue(stats.NetworkRx, before.NetworkRx)
				o.tx = deltaValue(stats.NetworkTx, before.NetworkTx)
				o.read = deltaValue(stats.BlockRead, before.BlockRead)
				o.write = deltaValue(stats.BlockWrite, before.BlockWrite)
			}
		}
		out = append(out, o)
		if c.ProjectID != "" {
			stackConditions[c.ProjectID] = append(stackConditions[c.ProjectID], condition)
			sum := stackMetrics[c.ProjectID]
			if o.cpu != nil {
				if sum.cpu == nil {
					sum.cpu = new(float64)
				}
				*sum.cpu += *o.cpu
			}
			if o.memory != nil {
				if sum.memory == nil {
					sum.memory = new(float64)
				}
				*sum.memory += *o.memory
			}
			sum.memoryLimit += o.memoryLimit
			sum.rx += o.rx
			sum.tx += o.tx
			sum.read += o.read
			sum.write += o.write
			stackMetrics[c.ProjectID] = sum
		}
	}
	for _, p := range sample.Projects {
		state := stackState(stackConditions[p.ID], p.Status)
		o := stackMetrics[p.ID]
		o.kind, o.key, o.state = "stack", p.ID, state
		out = append(out, o)
		c := dto.Condition{Kind: "stack", Name: p.Name, ProjectID: p.ID, Condition: state, Since: now, SampledAt: now}
		if prev, ok := priorByID["stack::"+p.ID]; ok && prev.Condition == state {
			c.Since = prev.Since
		}
		conditions = append(conditions, c)
	}
	return out, conditions
}
