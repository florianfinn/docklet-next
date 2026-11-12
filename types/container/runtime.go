// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-30
// License: Apache-2.0
// Deviations: Original process-local recreate result and recovery ports.

package container

import (
	"bytes"
	"context"
	"encoding/json"
)

type recreateSinkKey struct{}

func WithRecreateResultSink(ctx context.Context, sink func(RecreateResult)) context.Context {
	return context.WithValue(ctx, recreateSinkKey{}, sink)
}
func RecordRecreateResult(ctx context.Context, result RecreateResult) {
	if sink, ok := ctx.Value(recreateSinkKey{}).(func(RecreateResult)); ok {
		sink(result)
	}
}

// RecreateRecovery is a startup-only observation, never a replayable command.
type RecreateRecovery struct {
	RunID, Code string
	Result      *RecreateResult
}

// MarshalJSON preserves the edit contract's present-empty pointer sections
// when an outer transport uses encoding/json/v2's different omitempty rules.
func (e Edit) MarshalJSON() ([]byte, error) {
	type plain Edit
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	if err := encoder.Encode(plain(e)); err != nil {
		return nil, err
	}
	return bytes.TrimSuffix(buffer.Bytes(), []byte("\n")), nil
}
