// Source: docklet-next/issues/296
// Revision: a910be6a64e97a4ce3a422f12451a5301b115f7a
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original M13 closing-verification regressions.

package notification

import (
	"encoding/json/v2"
	"fmt"
	"github.com/florianfinn/docklet-next/types/imageupdate"
	dto "github.com/florianfinn/docklet-next/types/notification"
	"github.com/florianfinn/docklet-next/types/system"
	"github.com/stretchr/testify/require"
	"math"
	"strings"
	"testing"
	"time"
)

func TestLegacyReportKeepsExactIntegers(t *testing.T) {
	for _, n := range []uint64{1, 1<<53 + 1, math.MaxUint64} {
		t.Run(fmt.Sprint(n), func(t *testing.T) {
			p := dto.DispatchRequest{Kind: dto.DispatchKindPruneReport, PruneReport: &dto.DispatchPruneReport{Result: system.PruneAllResult{SpaceReclaimed: n}}}
			require.NoError(t, validateDispatch(&p), "valid uint64 must be accepted")
			require.Equal(t, n, p.PruneReport.Result.SpaceReclaimed, "numeric facts must be preserved")
		})
	}
}
func TestLegacyReportClampsEveryKindAndKeepsEmptyFields(t *testing.T) {
	for _, kind := range []dto.DispatchKind{dto.DispatchKindImageUpdate, dto.DispatchKindBatchImageUpdate, dto.DispatchKindContainerUpdate, dto.DispatchKindBatchContainerUpdate, dto.DispatchKindPruneReport} {
		t.Run(string(kind), func(t *testing.T) {
			text := strings.Repeat("ü", 100) + "\n"
			p := dto.DispatchRequest{Kind: kind}
			switch kind {
			case dto.DispatchKindImageUpdate:
				p.ImageUpdate = &dto.DispatchImageUpdate{ImageRef: text}
			case dto.DispatchKindBatchImageUpdate:
				p.BatchImageUpdate = &dto.DispatchBatchImageUpdate{Updates: map[string]*imageupdate.Response{"image": {Error: text}}}
			case dto.DispatchKindContainerUpdate:
				p.ContainerUpdate = &dto.DispatchContainerUpdate{ContainerName: text}
			case dto.DispatchKindBatchContainerUpdate:
				p.BatchContainerUpdate = &dto.DispatchBatchContainerUpdate{Updates: make([]dto.DispatchContainerUpdate, 300)}
			case dto.DispatchKindPruneReport:
				p.PruneReport = &dto.DispatchPruneReport{Result: system.PruneAllResult{Errors: []string{"one\ntwo"}}}
			}
			require.NoError(t, validateDispatch(&p))
			b, e := json.Marshal(p)
			require.NoError(t, e)
			require.NotContains(t, string(b), strings.Repeat("ü", 65))
			if p.BatchContainerUpdate != nil {
				require.Len(t, p.BatchContainerUpdate.Updates, 256)
			}
			if p.PruneReport != nil {
				require.Equal(t, "one two", p.PruneReport.Result.Errors[0])
			}
		})
	}
	t.Run("empty-fields", func(t *testing.T) {
		f := false
		s := ""
		at := time.Date(2026, 10, 30, 1, 2, 3, 123456789, time.UTC)
		for _, p := range []dto.DispatchRequest{
			{Kind: dto.DispatchKindPruneReport, PruneReport: &dto.DispatchPruneReport{Result: system.PruneAllResult{ImageSpaceReclaimedKnown: &f, ActivityID: &s, ImagesDeleted: []string{}}}},
			{Kind: dto.DispatchKindImageUpdate, ImageUpdate: &dto.DispatchImageUpdate{ImageRef: "example.com/app", UpdateInfo: imageupdate.Response{CheckTime: at, CheckedAt: at}}},
		} {
			b, e := json.Marshal(p)
			require.NoError(t, e)
			require.NoError(t, validateDispatch(&p))
			after, e := json.Marshal(p)
			require.NoError(t, e)
			require.JSONEq(t, string(b), string(after))
		}
	})
	for _, n := range []int{256, 257} {
		t.Run(fmt.Sprintf("maps-%d", n), func(t *testing.T) {
			m := map[string]*imageupdate.Response{}
			for i := 0; i < n; i++ {
				m[fmt.Sprint(i)] = &imageupdate.Response{}
			}
			p := dto.DispatchRequest{Kind: dto.DispatchKindBatchImageUpdate, BatchImageUpdate: &dto.DispatchBatchImageUpdate{Updates: m}}
			if n > 256 {
				require.Error(t, validateDispatch(&p))
			} else {
				require.NoError(t, validateDispatch(&p))
			}
		})
	}
	p := dto.DispatchRequest{Kind: dto.DispatchKindBatchImageUpdate, BatchImageUpdate: &dto.DispatchBatchImageUpdate{Updates: map[string]*imageupdate.Response{strings.Repeat("x", 129): {}}}}
	require.Error(t, validateDispatch(&p))
	for _, s := range []string{strings.Repeat("x", 129), "data\n"} {
		p = dto.DispatchRequest{Kind: dto.DispatchKindBackupFailed, BackupFailed: &dto.DispatchBackupFailed{EventID: "one", Operation: "backup", Volume: s, Reason: "volume.backup.failed"}}
		require.Error(t, validateDispatch(&p))
		p = dto.DispatchRequest{Kind: dto.DispatchKindAutoHeal, AutoHeal: &dto.DispatchAutoHeal{EventID: "one", Outcome: "restarted", Container: s, WindowMinutes: 30}}
		require.Error(t, validateDispatch(&p))
	}
}
