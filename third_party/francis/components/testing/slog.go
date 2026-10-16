// Source: italypaleale/francis/components/testing/slog.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package comptesting

import (
	"context"
	"log/slog"

	"k8s.io/utils/clock"
)

// SlogClockHandler is a custom slog.Handler that uses a mockable clock as time source.
type SlogClockHandler struct {
	slog.Handler

	clock clock.Clock
}

func NewSlogClockHandler(h slog.Handler, clock clock.Clock) *SlogClockHandler {
	return &SlogClockHandler{
		Handler: h,
		clock:   clock,
	}
}

func (h *SlogClockHandler) Handle(ctx context.Context, r slog.Record) error {
	// Replace the default timestamp with the one from our clock
	r.Time = h.clock.Now()
	return h.Handler.Handle(ctx, r)
}
