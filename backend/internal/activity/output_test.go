// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package activity

import (
	"encoding/json/v2"
	"strings"
	"testing"
	"time"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestOutputLimitKeepsNewestLinesInternal(t *testing.T) {
	line := func(text string) activitytypes.OutputLine {
		return activitytypes.OutputLine{Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC), Stream: "stdout", Text: text}
	}
	first := line(strings.Repeat("a", 140_000))
	second := line(strings.Repeat("b", 140_000))
	third := line("newest")
	output, truncated := appendOutputLinesInternal(nil, []activitytypes.OutputLine{first, second, third})
	require.True(t, truncated)
	require.Equal(t, []activitytypes.OutputLine{second, third}, []activitytypes.OutputLine(output))
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxSerializedOutputBytes)
}

func TestOutputLimitKeepsUTF8TailOfOversizedLineInternal(t *testing.T) {
	text := "head:" + strings.Repeat("ä", 150_000) + ":tail"
	output, truncated := appendOutputLinesInternal(nil, []activitytypes.OutputLine{{
		Timestamp: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
		Stream:    "stderr", Text: text,
	}})
	require.True(t, truncated)
	require.Len(t, output, 1)
	require.NotContains(t, output[0].Text, "head:")
	require.True(t, strings.HasSuffix(output[0].Text, ":tail"))
	encoded, err := json.Marshal(output)
	require.NoError(t, err)
	require.LessOrEqual(t, len(encoded), maxSerializedOutputBytes)
}
