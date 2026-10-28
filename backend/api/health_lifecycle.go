// Source: docklet-next/issues/245
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original lifecycle liveness for the health routes.

package api

import "context"

// lifecycleLive reports whether the application lifecycle context is still running; health fails once it ends.
func lifecycleLive(ctx context.Context) func() bool {
	if ctx == nil {
		return nil
	}
	return func() bool { return ctx.Err() == nil }
}
