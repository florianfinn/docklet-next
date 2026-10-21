// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Failed executor tail separation and existing output retention bounds.

package activity

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"strings"
	"testing"
	"unicode/utf8"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
	"github.com/stretchr/testify/require"
)

func TestRuntimeFailedOutputPreservesPartialTailAndBounds(t *testing.T) {
	for _, large := range []bool{false, true} {
		name, diagnostic := "partial-tail", "manifest unknown: \"example\"\nregistry rejected image"
		if large {
			name, diagnostic = "bounded-error", strings.Repeat("界", 200000)
		}
		t.Run(name, func(t *testing.T) {
			f := b2Fixture(t, t.Context(), func(_ context.Context, _ RuntimeCommand, output io.Writer) error {
				_, err := io.WriteString(output, "pull progress without newline")
				require.NoError(t, err)
				return errors.New(diagnostic)
			})
			command := f.command()
			admitted, err := f.service.Admit(t.Context(), command)
			require.NoError(t, err)
			settled, err := f.service.WaitLocal(t.Context(), b2Ref(command, admitted.ActivityID))
			require.NoError(t, err)
			require.Equal(t, activitytypes.StatusFailed, settled.Status)
			require.Equal(t, "runtime.failed", settled.MessageCode)
			detail, err := f.store.svc.GetActivityDetail(t.Context(), "0", admitted.ActivityID, 100)
			require.NoError(t, err)
			require.NotEmpty(t, detail.Output)
			encoded, err := json.Marshal(detail.Output)
			require.NoError(t, err)
			require.LessOrEqual(t, len(encoded), maxSerializedOutputBytes)
			for _, line := range detail.Output {
				require.True(t, utf8.ValidString(line.Text))
			}
			if large {
				require.True(t, detail.OutputTruncated)
				require.Contains(t, detail.Output[len(detail.Output)-1].Text, "[runtime output truncated]")
			} else {
				require.Len(t, detail.Output, 2)
				require.Equal(t, "pull progress without newline", detail.Output[0].Text)
				var frame map[string]string
				require.NoError(t, json.Unmarshal([]byte(detail.Output[1].Text), &frame))
				require.Equal(t, diagnostic, frame["error"])
				require.Equal(t, "stderr", detail.Output[1].Stream)
				require.Equal(t, activitytypes.MessageLevelError, detail.Output[1].Level)
				require.False(t, detail.OutputTruncated)
			}
		})
	}
}
