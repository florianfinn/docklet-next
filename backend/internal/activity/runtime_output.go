// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-10-19
// License: Apache-2.0
// Deviations: Runtime capture ownership and bounded whole-drain wait; no socket output.

package activity

import (
	"context"
	"encoding/json/v2"
	"errors"
	"io"
	"time"

	activitylib "github.com/florianfinn/docklet-next/backend/pkg/libarcane/activity"
)

// The finite queue has 128 entries plus an active batch and a final tail. Six
// 10-second appends cover that backlog after producers join. This is a wait
// budget, never proof that a context-ignoring appender has stopped.
const runtimeOutputDrainWait = 60 * time.Second

func (s *RuntimeService) captureInternal(ctx context.Context, id string) (*activitylib.Writer, error) {
	return activitylib.NewRuntimeWriter(ctx, s.outputAppender, id, "")
}

// SDKs can return their only diagnostic as an error without writing progress.
// Keep that diagnostic in the same bounded capture, separate from coded reasons.
func runtimeCaptureFailureInternal(output *activitylib.Writer, workflowErr error) error {
	if workflowErr == nil {
		return nil
	}
	output.Flush() // Seal any unterminated progress line before the error frame.
	if err := json.MarshalWrite(output, map[string]string{"error": workflowErr.Error()}); err != nil {
		return err
	}
	_, err := io.WriteString(output, "\n")
	return err
}

func runtimeFinishOutputInternal(ctx context.Context, output *activitylib.Writer) error {
	wait, cancel := context.WithTimeout(context.WithoutCancel(ctx), runtimeOutputDrainWait)
	defer cancel()
	err := output.Finish(wait)
	// The owner remains live even beyond the wait budget. Go cannot kill an
	// appender ignoring its context; actual join is required before settlement.
	<-output.Done()
	return errors.Join(err, output.Finish(context.WithoutCancel(ctx)))
}
