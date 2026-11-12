// Source: docklet-next/issues/401
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original injected disk sampler, persisted capacity and source-epoch tests.

package history

import (
	"bytes"
	"context"
	"errors"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/backend/internal/environment"
	dto "github.com/florianfinn/docklet-next/types/history"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/stretchr/testify/require"
)

func TestDiskFallbackPersistsCapacityAndWarningEpisodes(t *testing.T) {
	for _, failure := range []string{"missing", "zero-total", "both-missing", "root-zero-total"} {
		t.Run(failure, func(t *testing.T) {
			svc, host, now := samplerFixture(t)
			now = now.Truncate(2 * time.Hour)
			var logs bytes.Buffer
			configuredOK := false
			reads := []string{}
			sampler := diskSampler{hostID: host.ID, logger: slog.New(slog.NewJSONHandler(&logs, nil))}
			sampler.read = func(_ context.Context, path string) (*disk.UsageStat, error) {
				reads = append(reads, path)
				if path == "/example" && configuredOK {
					return &disk.UsageStat{Used: 20, Total: 100}, nil
				}
				if path == "/example" && failure == "zero-total" {
					return &disk.UsageStat{}, nil
				}
				if path == "/" && failure == "root-zero-total" {
					return &disk.UsageStat{}, nil
				}
				if path == "/" && failure != "both-missing" {
					return &disk.UsageStat{Used: 40, Total: 200}, nil
				}
				return nil, errors.New("disk unavailable")
			}
			svc.fetch = func(ctx context.Context, _ environment.Environment) (dto.Sample, error) {
				h, err := sampleHostWithDisk(ctx, "/example", &sampler)
				return dto.Sample{Host: h, Containers: []dto.ContainerSample{}, Projects: []dto.ProjectSample{}}, err
			}
			both := failure == "both-missing" || failure == "root-zero-total"
			for minute := range 2 {
				require.NoError(t, svc.SampleHost(t.Context(), host, now.Add(time.Duration(minute)*time.Minute)))
				capacities, err := svc.Capacity(t.Context(), now.Add(time.Duration(minute)*time.Minute))
				require.NoError(t, err)
				if both {
					require.Empty(t, capacities)
				} else {
					require.Len(t, capacities, 1)
					require.Equal(t, "/", capacities[0].DiskSource)
					require.Equal(t, 40., capacities[0].UsedBytes)
					require.Equal(t, 200., capacities[0].TotalBytes)
				}
			}
			require.Equal(t, []string{"/example", "/", "/example", "/"}, reads)
			require.Equal(t, 1, strings.Count(logs.String(), `"code":"history.disk-fallback"`), "one warning while configured path keeps failing")
			require.Contains(t, logs.String(), `"path":"/example"`)
			require.Contains(t, logs.String(), `"hostId":"host"`)
			require.Contains(t, logs.String(), `"level":"WARN"`)
			configuredOK = true
			require.NoError(t, svc.SampleHost(t.Context(), host, now.Add(2*time.Minute)))
			capacities, err := svc.Capacity(t.Context(), now.Add(2*time.Minute))
			require.NoError(t, err)
			require.Len(t, capacities, 1)
			require.Equal(t, "/example", capacities[0].DiskSource)
			require.Equal(t, 20., capacities[0].UsedBytes)
			configuredOK = false
			require.NoError(t, svc.SampleHost(t.Context(), host, now.Add(3*time.Minute)))
			capacities, err = svc.Capacity(t.Context(), now.Add(3*time.Minute))
			require.NoError(t, err)
			require.Len(t, capacities, 1)
			if both {
				// No disk values in this sample keeps the last measured source and values in the bucket.
				require.Equal(t, "/example", capacities[0].DiskSource)
				require.Equal(t, 20., capacities[0].UsedBytes)
				require.Equal(t, 100., capacities[0].TotalBytes)
			} else {
				require.Equal(t, "/", capacities[0].DiskSource)
			}
			require.Equal(t, 2, strings.Count(logs.String(), `"code":"history.disk-fallback"`), "success resets warning suppression")
			var rows []bucketRow
			require.NoError(t, svc.db.Where("kind='host' AND tier=120").Find(&rows).Error)
			require.Len(t, rows, 1)
			require.Equal(t, capacities[0].DiskSource, rows[0].DiskSource)
		})
	}
}

func TestDiskFallbackRootFailureDoesNotRepeatRead(t *testing.T) {
	var logs bytes.Buffer
	calls := 0
	sampler := diskSampler{hostID: "0", logger: slog.New(slog.NewJSONHandler(&logs, nil)), read: func(_ context.Context, path string) (*disk.UsageStat, error) {
		calls++
		require.Equal(t, "/", path)
		return nil, errors.New("unavailable")
	}}
	for range 2 {
		var h dto.HostSample
		sampler.sample(t.Context(), "/", &h)
		require.Empty(t, h.DiskSource)
		require.Zero(t, h.DiskTotal)
		require.Zero(t, h.DiskUsed)
	}
	require.Equal(t, 2, calls)
	require.Equal(t, 1, strings.Count(logs.String(), "history.disk-fallback"))
}

func TestCapacityReusesEarlierEpochOfCurrentSource(t *testing.T) {
	svc, host, now := samplerFixture(t)
	now = now.Truncate(2 * time.Hour)
	for i, source := range []string{"/example", "/", "/example"} {
		at := now.Add(time.Duration(i-2) * 24 * time.Hour).Unix()
		used := float64(20 + i*10)
		if source == "/" {
			at = now.Add(-12 * time.Hour).Unix()
			used = 900
		}
		require.NoError(t, svc.db.Create(&bucketRow{Kind: "host", HostID: host.ID, EntityKey: host.ID, Tier: 120, BucketStart: at, LastMinute: at, MinutesRunning: 1, DiskSource: source, DiskUsedLast: used, DiskTotalLast: 1000}).Error)
	}
	capacities, err := svc.Capacity(t.Context(), now)
	require.NoError(t, err)
	require.Len(t, capacities, 1)
	c := capacities[0]
	require.Equal(t, "/example", c.DiskSource)
	require.False(t, c.GrowthUnavailable, "earlier same-source epoch contributes")
	require.NotNil(t, c.GrowthBytesPerDay)
	require.Equal(t, 10., *c.GrowthBytesPerDay, "root-source bucket must be excluded")
	require.Equal(t, 96., *c.DaysUntilFull)
}

func TestDiskSamplerUnsetPathMeasuresRootWithoutWarning(t *testing.T) {
	var logs bytes.Buffer
	reads := []string{}
	sampler := diskSampler{hostID: "0", logger: slog.New(slog.NewJSONHandler(&logs, nil))}
	sampler.read = func(_ context.Context, path string) (*disk.UsageStat, error) {
		reads = append(reads, path)
		return &disk.UsageStat{Used: 40, Total: 200}, nil
	}
	var out dto.HostSample
	sampler.sample(t.Context(), "", &out)
	require.Equal(t, "/", out.DiskSource)
	require.Equal(t, []string{"/"}, reads)
	require.Empty(t, logs.String())
}
