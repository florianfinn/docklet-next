// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original one-shot Docker and host history sampling; disk source fallback and suppression of repeated warnings.

package history

import (
	"context"
	"encoding/json/jsontext"
	"encoding/json/v2"
	"errors"
	"slices"
	"strings"
	"time"

	"github.com/containerd/errdefs"
	"github.com/florianfinn/docklet-next/backend/internal/database"
	"github.com/florianfinn/docklet-next/backend/internal/docker"
	"github.com/florianfinn/docklet-next/backend/internal/project"
	"github.com/florianfinn/docklet-next/backend/internal/runtimeintent"
	"github.com/florianfinn/docklet-next/backend/internal/settings"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/client"
	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/mem"
	gnet "github.com/shirou/gopsutil/v4/net"
	"go.getarcane.app/streams/stats"
	"go.getarcane.app/sys/cgroup"
	"golang.org/x/sync/errgroup"
)

type Producer struct {
	db               *database.DB
	getClient        func(context.Context) (*client.Client, error)
	hostSample       func(context.Context) (dto.HostSample, error)
	dockerHostMemory func(context.Context) (uint64, uint64, bool)
	inspectTimeout   time.Duration
}

func NewProducer(db *database.DB, dockerService *docker.DockerClientService, settingsService *settings.SettingsService) *Producer {
	p := &Producer{db: db, getClient: dockerService.GetClient, inspectTimeout: 3 * time.Second}
	p.hostSample = func(ctx context.Context) (dto.HostSample, error) {
		h, err := sampleHost(ctx, settingsService.GetSettingsConfig().DiskUsagePath.Value)
		if err == nil && p.dockerHostMemory != nil && cgroup.IsDockerContainer() {
			if used, total, ok := p.dockerHostMemory(ctx); ok {
				h.MemoryUsed, h.MemoryTotal = float64(used), float64(total)
			}
		}
		return h, err
	}
	return p
}

// SetDockerHostMemoryProvider reuses the live system-stat host accounting boundary.
func (p *Producer) SetDockerHostMemoryProvider(provider func(context.Context) (uint64, uint64, bool)) {
	p.dockerHostMemory = provider
}
func sampleHost(ctx context.Context, path string) (dto.HostSample, error) {
	return sampleHostWithDisk(ctx, path, &localHistoryDisk)
}

func sampleHostWithDisk(ctx context.Context, path string, sampler *diskSampler) (dto.HostSample, error) {
	var h dto.HostSample
	times, err := cpu.TimesWithContext(ctx, false)
	if err != nil {
		return h, err
	}
	for _, t := range times {
		h.CPUTotal += t.Total()
		h.CPUBusy += t.Total() - t.Idle - t.Iowait
	}
	memory, err := mem.VirtualMemoryWithContext(ctx)
	if err != nil {
		return h, err
	}
	used := memory.Used
	used -= min(used, cgroup.ZFSARCReclaimable())
	h.MemoryUsed, h.MemoryTotal = float64(used), float64(memory.Total)
	sampler.sample(ctx, path, &h)
	interfaces, err := gnet.InterfacesWithContext(ctx)
	if err != nil {
		return h, err
	}
	loopback := map[string]bool{}
	for _, i := range interfaces {
		for _, f := range i.Flags {
			if f == "loopback" {
				loopback[i.Name] = true
			}
		}
	}
	counters, err := gnet.IOCountersWithContext(ctx, true)
	if err != nil {
		return h, err
	}
	for _, c := range counters {
		if !loopback[c.Name] {
			h.NetworkRx += float64(c.BytesRecv)
			h.NetworkTx += float64(c.BytesSent)
		}
	}
	return h, nil
}

func (p *Producer) Sample(ctx context.Context) (dto.Sample, error) {
	out := dto.Sample{Containers: []dto.ContainerSample{}, Projects: []dto.ProjectSample{}}
	var err error
	out.Host, err = p.hostSample(ctx)
	if err != nil {
		return out, err
	}
	cli, err := p.getClient(ctx)
	if err != nil {
		return out, err
	}
	list, err := cli.ContainerList(ctx, client.ContainerListOptions{All: true})
	if err != nil {
		return out, err
	}
	if len(list.Items) > 2000 {
		return out, dto.Fail("history.sample-invalid", 502)
	}
	var projects []project.Project
	if err = p.db.WithContext(ctx).Find(&projects).Error; err != nil {
		return out, err
	}
	projectIDs := map[string]string{}
	for _, v := range projects {
		name := v.Name
		if v.ComposeProjectName != nil && *v.ComposeProjectName != "" {
			name = *v.ComposeProjectName
		}
		projectIDs[name] = v.ID
		out.Projects = append(out.Projects, dto.ProjectSample{ID: v.ID, Name: v.Name, Status: string(v.Status)})
	}
	out.Containers = make([]dto.ContainerSample, len(list.Items))
	uninspected := make([]bool, len(list.Items))
	group, groupCtx := errgroup.WithContext(ctx)
	group.SetLimit(16)
	for i, c := range list.Items {
		group.Go(func() error {
			callCtx, cancel := context.WithTimeout(groupCtx, p.inspectTimeout)
			defer cancel()
			inspected, err := cli.ContainerInspect(callCtx, c.ID, client.ContainerInspectOptions{})
			if err != nil {
				// A container removed since the list, or one slow inspect, skips only that container
				// for this minute; any other failure fails the host sample.
				if groupCtx.Err() != nil {
					return groupCtx.Err()
				}
				if errdefs.IsNotFound(err) {
					return nil
				}
				if errors.Is(callCtx.Err(), context.DeadlineExceeded) {
					uninspected[i] = true
					return nil
				}
				return err
			}
			v := inspected.Container
			item := dto.ContainerSample{ID: c.ID, Name: strings.TrimPrefix(v.Name, "/")}
			if v.Config != nil {
				item.ProjectName = v.Config.Labels["com.docker.compose.project"]
				item.ProjectID = projectIDs[item.ProjectName]
				item.Service = v.Config.Labels["com.docker.compose.service"]
			}
			if v.State == nil {
				return dto.Fail("history.sample-invalid", 502)
			}
			item.State = string(v.State.Status)
			item.ExitCode = v.State.ExitCode
			item.OOMKilled = v.State.OOMKilled
			item.StartedAt, item.FinishedAt = v.State.StartedAt, v.State.FinishedAt
			item.RestartCount = v.RestartCount
			if v.State.Health != nil {
				item.Health = string(v.State.Health.Status)
			}
			if item.State == "running" {
				data, statsErr := cli.ContainerStats(callCtx, c.ID, client.ContainerStatsOptions{Stream: false, IncludePreviousSample: false})
				item.StatsMissing = statsErr != nil
				if statsErr == nil {
					var raw container.StatsResponse
					statsErr = json.UnmarshalDecode(jsontext.NewDecoder(data.Body), &raw)
					_ = data.Body.Close()
					item.StatsMissing = statsErr != nil
					if statsErr == nil {
						item.Stats = sampleCounters(raw)
					}
				}
			}
			out.Containers[i] = item
			return nil
		})
	}
	if err = group.Wait(); err != nil {
		return out, err
	}
	out.Containers = slices.DeleteFunc(out.Containers, func(c dto.ContainerSample) bool { return c.ID == "" })
	for i, skipped := range uninspected {
		if skipped {
			out.Uninspected = append(out.Uninspected, list.Items[i].ID)
		}
	}
	if err = ctx.Err(); err != nil {
		return out, err
	}
	if err = p.applyIntents(ctx, &out); err != nil {
		return out, err
	}
	return out, Validate(out)
}
func sampleCounters(raw container.StatsResponse) *dto.Counters {
	built := stats.BuildSample(raw)
	c := &dto.Counters{CPUTotal: float64(raw.CPUStats.CPUUsage.TotalUsage), CPUSystem: float64(raw.CPUStats.SystemUsage), MemoryUsage: float64(built.MemoryUsageBytes), MemoryLimit: float64(raw.MemoryStats.Limit)}
	for _, n := range raw.Networks {
		c.NetworkRx += float64(n.RxBytes)
		c.NetworkTx += float64(n.TxBytes)
	}
	for _, b := range raw.BlkioStats.IoServiceBytesRecursive {
		switch strings.ToLower(b.Op) {
		case "read":
			c.BlockRead += float64(b.Value)
		case "write":
			c.BlockWrite += float64(b.Value)
		}
	}
	return c
}
func (p *Producer) applyIntents(ctx context.Context, sample *dto.Sample) error {
	var rows []runtimeintent.Row
	if err := p.db.WithContext(ctx).Find(&rows).Error; err != nil {
		return err
	}
	current := map[string]*dto.ContainerSample{}
	unknown := map[string]bool{}
	for _, id := range sample.Uninspected {
		unknown[id] = true
	}
	for i := range sample.Containers {
		c := &sample.Containers[i]
		current[c.ID] = c
	}
	for _, r := range rows {
		if unknown[r.ContainerID] {
			continue
		}
		c, exists := current[r.ContainerID]
		if exists {
			started, err := time.Parse(time.RFC3339Nano, c.StartedAt)
			if err == nil && !started.After(r.RecordedAt) {
				c.StopIntent = true
				continue
			}
		}
		if err := p.db.WithContext(ctx).Delete(&runtimeintent.Row{}, "container_id=?", r.ContainerID).Error; err != nil {
			return err
		}
	}
	return nil
}
