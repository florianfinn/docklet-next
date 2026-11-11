// Source: docklet-next/issues/401
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original owning-host disk fallback with one warning per failure episode.

package history

import (
	"context"
	"log/slog"
	"sync"

	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/shirou/gopsutil/v4/disk"
)

type diskSampler struct {
	mu      sync.Mutex
	failing bool
	hostID  string
	read    func(context.Context, string) (*disk.UsageStat, error)
	logger  *slog.Logger
}

// Each process measures its own host, addressed locally as environment 0 on hub and agents.
var localHistoryDisk = diskSampler{hostID: "0", read: disk.UsageWithContext}

func (s *diskSampler) sample(ctx context.Context, path string, out *dto.HostSample) {
	s.mu.Lock()
	defer s.mu.Unlock()
	usage, err := s.read(ctx, path)
	if err == nil && usage != nil && usage.Total > 0 {
		s.failing = false
		out.DiskSource, out.DiskUsed, out.DiskTotal = path, float64(usage.Used), float64(usage.Total)
		return
	}
	if !s.failing {
		logger := s.logger
		if logger == nil {
			logger = slog.Default()
		}
		logger.WarnContext(ctx, "History disk measurement fell back", "code", "history.disk-fallback", "path", path, "hostId", s.hostID)
		s.failing = true
	}
	if path == "/" {
		return
	}
	usage, err = s.read(ctx, "/")
	if err == nil && usage != nil && usage.Total > 0 {
		out.DiskSource, out.DiskUsed, out.DiskTotal = "/", float64(usage.Used), float64(usage.Total)
	}
}
