// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original D4 state reader boundary for the shell capability.

package containershell

import (
	"context"
	"github.com/danielgtaylor/huma/v2"
)

type StateReader func(context.Context, string, string) string
type stateReaderKey struct{}

func WithStateReader(ctx context.Context, reader StateReader) context.Context {
	return context.WithValue(ctx, stateReaderKey{}, reader)
}
func Availability(ctx context.Context, env, containerID string) string {
	reader, _ := ctx.Value(stateReaderKey{}).(StateReader)
	if reader == nil {
		return "capability.unavailable"
	}
	return reader(ctx, env, containerID)
}
func CapabilityContext(reader StateReader) func(huma.Context, func(huma.Context)) {
	return func(ctx huma.Context, next func(huma.Context)) {
		if ctx.Operation().OperationID == "get-capabilities" {
			ctx = huma.WithContext(ctx, WithStateReader(ctx.Context(), reader))
		}
		next(ctx)
	}
}
