// Source: docklet-next/issues/327
// Revision: b9b59ae92f1b09eb828940860971002eab9267b6
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original M16 bind identity and admission rules; M16 bind mounts and backup completion.

package volume

import (
	"context"
	"errors"
	"github.com/florianfinn/docklet-next/backend/internal/volume/bindpath"
	dto "github.com/florianfinn/docklet-next/types/backup"
	volumetypes "github.com/florianfinn/docklet-next/types/volume"
	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/mount"
	"github.com/moby/moby/client"
	"io/fs"
	"slices"
	"strings"
	"sync"
	"time"
)

type bindInventory struct {
	mu       sync.Mutex
	kinds    map[string]string
	scans    map[string]*bindScan
	measured map[string]volumetypes.BindMeasurement
}

func (s *VolumeService) bindState() *bindInventory {
	s.helperMu.Lock()
	defer s.helperMu.Unlock()
	if s.binds == nil {
		s.binds = &bindInventory{kinds: map[string]string{}, scans: map[string]*bindScan{}, measured: map[string]volumetypes.BindMeasurement{}}
	}
	return s.binds
}
func (s *VolumeService) ListBinds(ctx context.Context) ([]volumetypes.Bind, error) {
	cli, err := s.dockerService.GetClient(ctx)
	if err != nil {
		return nil, err
	}
	rows, err := s.dockerService.ListContainers(ctx)
	if err != nil {
		return nil, err
	}
	return s.bindsFromContainers(ctx, cli, rows)
}
func (s *VolumeService) bindsFromContainers(ctx context.Context, cli *client.Client, rows []container.Summary) ([]volumetypes.Bind, error) {
	info, err := cli.Info(ctx, client.InfoOptions{})
	if err != nil {
		return nil, err
	}
	targets, err := bindpath.TargetPaths(ctx, s.db)
	if err != nil {
		return nil, err
	}
	bySource := map[string]*volumetypes.Bind{}
	state := s.bindState()
	for _, c := range rows {
		if bindpath.Self(c) || bindpath.Helper(c) {
			continue
		}
		for _, m := range c.Mounts {
			if m.Type != mount.TypeBind {
				continue
			}
			key := volumetypes.BindKey(m.Source)
			row := bySource[m.Source]
			if row == nil {
				row = &volumetypes.Bind{Key: key, Source: m.Source, Kind: "unknown", Eligible: true, Consumers: []volumetypes.BindConsumer{}}
				if err := volumetypes.ValidateName(key); err != nil {
					row.Eligible = false
					row.Reason = "volume.bind.invalid-key"
				} else if err := bindpath.Rules(m.Source, info.Info.DockerRootDir, rows, targets, true); err != nil {
					var p *dto.Problem
					if errors.As(err, &p) {
						row.Eligible = false
						row.Reason = p.Code
					}
				}
				bySource[m.Source] = row
			}
			name := c.ID
			if len(c.Names) > 0 {
				name = strings.TrimPrefix(c.Names[0], "/")
			}
			row.Consumers = append(row.Consumers, volumetypes.BindConsumer{ContainerID: c.ID, Name: name, Destination: m.Destination, ReadOnly: !m.RW, Running: c.State == container.StateRunning})
			cacheKey := c.ID + "\x00" + m.Destination
			state.mu.Lock()
			kind, known := state.kinds[cacheKey]
			state.mu.Unlock()
			if !known {
				kind = "unknown"
				if stat, err := cli.ContainerStatPath(ctx, c.ID, client.ContainerStatPathOptions{Path: m.Destination}); err == nil {
					mode := fs.FileMode(stat.Stat.Mode)
					switch {
					case mode.IsDir():
						kind = "directory"
					case mode.IsRegular():
						kind = "file"
					default:
						kind = "special"
					}
				}
				state.mu.Lock()
				state.kinds[cacheKey] = kind
				state.mu.Unlock()
			}
			if row.Kind == "unknown" {
				if kind == "special" {
					row.Eligible = false
					row.Reason = "volume.bind.unsupported-type"
				} else {
					row.Kind = kind
				}
			}
		}
	}
	result := make([]volumetypes.Bind, 0, len(bySource))
	for _, row := range bySource {
		state.mu.Lock()
		measured, ok := state.measured[row.Key]
		state.mu.Unlock()
		if ok {
			row.MeasuredSize = &measured.MeasuredSize
			row.MeasuredAt = &measured.MeasuredAt
			row.MeasuredPartial = &measured.MeasuredPartial
		}
		if s.db != nil {
			var last struct {
				Size      int64
				CreatedAt time.Time
			}
			q := s.db.WithContext(ctx).Table("volume_backups").Select("size,created_at").Where("volume_name = ? AND status = ?", row.Key, "succeeded").Order("created_at DESC").Limit(1).Find(&last)
			if q.Error != nil {
				return nil, q.Error
			}
			if q.RowsAffected > 0 {
				if last.Size >= 0 {
					row.LastSnapshotSize = &last.Size
				}
				row.LastSnapshotAt = &last.CreatedAt
			}
		}
		slices.SortFunc(row.Consumers, func(a, b volumetypes.BindConsumer) int {
			if a.ContainerID != b.ContainerID {
				return strings.Compare(a.ContainerID, b.ContainerID)
			}
			return strings.Compare(a.Destination, b.Destination)
		})
		result = append(result, *row)
	}
	slices.SortFunc(result, func(a, b volumetypes.Bind) int { return strings.Compare(a.Source, b.Source) })
	return result, nil
}
