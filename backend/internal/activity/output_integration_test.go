// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original PostgreSQL retention and detail regression test.

package activity

import (
	"strings"
	"testing"
	"time"

	"github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
	"gorm.io/gorm"
)

func TestRawOutputDetailAndRetentionInternal(t *testing.T) {
	ctx := t.Context()
	service := NewActivityService(setupActivityServiceTestDBInternal(t), nil)
	finished, err := service.StartActivity(ctx, StartActivityRequest{EnvironmentID: "0", Type: activity.TypeImagePull})
	require.NoError(t, err)
	line := activity.OutputLine{Timestamp: time.Now().UTC(), Stream: "stdout", Text: "raw output", Step: "pull"}
	_, err = service.AppendMessages(ctx, finished.ID, []AppendActivityMessageRequest{{Level: activity.MessageLevelInfo, Message: "normalized", Output: &line}})
	require.NoError(t, err)
	firstRead, err := service.GetActivityDetail(ctx, "0", finished.ID, 10)
	require.NoError(t, err)
	require.Equal(t, []activity.OutputLine{line}, firstRead.Output)
	secondRead, err := service.GetActivityDetail(ctx, "0", finished.ID, 10)
	require.NoError(t, err)
	require.Equal(t, firstRead.Output, secondRead.Output)
	largeOld := activity.OutputLine{Timestamp: line.Timestamp, Stream: "stdout", Text: strings.Repeat("a", 140_000)}
	largeNew := activity.OutputLine{Timestamp: line.Timestamp, Stream: "stdout", Text: strings.Repeat("b", 140_000)}
	_, err = service.AppendMessages(ctx, finished.ID, []AppendActivityMessageRequest{{Output: &largeOld}, {Output: &largeNew}})
	require.NoError(t, err)
	truncatedRead, err := service.GetActivityDetail(ctx, "0", finished.ID, 10)
	require.NoError(t, err)
	require.True(t, truncatedRead.OutputTruncated)
	require.Equal(t, []activity.OutputLine{largeNew}, truncatedRead.Output)
	_, err = service.CompleteActivity(ctx, finished.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)

	newer, err := service.StartActivity(ctx, StartActivityRequest{EnvironmentID: "0", Type: activity.TypeImagePull})
	require.NoError(t, err)
	_, err = service.CompleteActivity(ctx, newer.ID, activity.StatusSuccess, "done", nil)
	require.NoError(t, err)
	active, err := service.StartActivity(ctx, StartActivityRequest{EnvironmentID: "0", Type: activity.TypeImagePull})
	require.NoError(t, err)
	_, err = service.AppendMessages(ctx, active.ID, []AppendActivityMessageRequest{{Output: &line}})
	require.NoError(t, err)

	deleted, err := service.PruneHistory(ctx, 1)
	require.NoError(t, err)
	require.Equal(t, int64(1), deleted)
	_, err = service.GetActivityDetail(ctx, "0", finished.ID, 10)
	require.ErrorIs(t, err, gorm.ErrRecordNotFound)
	activeDetail, err := service.GetActivityDetail(ctx, "0", active.ID, 10)
	require.NoError(t, err)
	require.Equal(t, []activity.OutputLine{line}, activeDetail.Output)
}
