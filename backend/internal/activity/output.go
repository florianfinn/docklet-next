// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: dfdfa084478d327818d8c57b0973d638f80861f6
// Retrieved: 2026-10-15
// License: Apache-2.0
// Deviations: Original implementation for issue 38.
package activity

import (
	"encoding/json/v2"

	activitytypes "github.com/florianfinn/docklet-next/types/activity"
)

const maxSerializedOutputBytes = 256 * 1024

func serializedOutputLineSizeInternal(line activitytypes.OutputLine) int {
	encoded, _ := json.Marshal(line)
	return len(encoded)
}

func appendOutputLinesInternal(existing OutputLines, incoming []activitytypes.OutputLine) (OutputLines, bool) {
	lines := append(OutputLines{}, existing...)
	size := 2
	for _, line := range lines {
		size += serializedOutputLineSizeInternal(line)
	}
	if len(lines) > 1 {
		size += len(lines) - 1
	}
	truncated := false
	for _, line := range incoming {
		lineSize := serializedOutputLineSizeInternal(line)
		if lineSize+2 > maxSerializedOutputBytes {
			truncated = true
			line.Step = ""
			runes := []rune(line.Text)
			low, high := 0, len(runes)
			for low < high {
				mid := (low + high + 1) / 2
				line.Text = string(runes[len(runes)-mid:])
				if serializedOutputLineSizeInternal(line)+2 <= maxSerializedOutputBytes {
					low = mid
				} else {
					high = mid - 1
				}
			}
			line.Text = string(runes[len(runes)-low:])
			lineSize = serializedOutputLineSizeInternal(line)
		}
		if len(lines) > 0 {
			size++
		}
		lines = append(lines, line)
		size += lineSize
		for size > maxSerializedOutputBytes && len(lines) > 1 {
			size -= serializedOutputLineSizeInternal(lines[0]) + 1
			lines = lines[1:]
			truncated = true
		}
	}
	return lines, truncated
}
