// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: CLI completion proof at the existing NDJSON parser boundary.

package projects

import (
	"bytes"
	"errors"
	"io"
	"strings"
	"testing"

	"github.com/florianfinn/docklet-next/cli/internal/output"
	"github.com/stretchr/testify/require"
)

func TestRuntimeOperationStreamRequiresDone(t *testing.T) {
	for _, tc := range []struct{ name, stream, message, progress string }{
		{"empty", "", "interrupted before completion", ""},
		{"started", "{\"type\":\"activity\",\"activityId\":\"example-run\"}\n", "interrupted before completion", ""},
		{"progress", "{\"log\":\"pulling image\"}\n", "interrupted before completion", "pulling image\n"},
		{"invalid", "invalid JSON\n", "interrupted before completion", ""},
		{"terminal-error", "{\"log\":\"starting\"}\n{\"error\":\"runtime.failed\",\"done\":true}\n", "runtime.failed", "starting\n"},
		{"done", "{\"log\":\"started\"}\n{\"done\":true}\n", "", "started\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			output.Configure(&stdout, &stderr, true, true, "runtime")
			defer output.Configure(nil, nil, false, true, "")
			err := printOperationStreamInternal(strings.NewReader(tc.stream))
			if tc.message == "" {
				require.NoError(t, err)
			} else {
				require.ErrorContains(t, err, tc.message)
			}
			require.Equal(t, tc.progress, stdout.String())
		})
	}
}

type runtimeFailedStream struct{}

func (runtimeFailedStream) Read([]byte) (int, error) { return 0, errors.New("synthetic read failure") }

func TestRuntimeOperationStreamPreservesReadError(t *testing.T) {
	require.ErrorContains(t, printOperationStreamInternal(io.MultiReader(strings.NewReader("{\"log\":\"progress\"}\n"), runtimeFailedStream{})), "synthetic read failure")
}
