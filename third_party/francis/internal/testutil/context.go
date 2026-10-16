// Source: italypaleale/francis/internal/testutil/context.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider test support restored for default tests; reserved example addresses; oversized suite split without behavior changes.

package testutil

import (
	"context"
	"sync/atomic"
)

//nolint:containedctx
type contextDoneNotifier struct {
	context.Context

	doneCalled *atomic.Pointer[chan struct{}]
}

func NewContextDoneNotifier(parentCtx context.Context) *contextDoneNotifier {
	doneCalled := &atomic.Pointer[chan struct{}]{}
	doneCalled.Store(new(make(chan struct{})))
	return &contextDoneNotifier{
		Context:    parentCtx,
		doneCalled: doneCalled,
	}
}

func (c *contextDoneNotifier) WaitForDone() {
	doneCalled := c.doneCalled.Load()
	if doneCalled != nil {
		<-*doneCalled
	}
}

func (c *contextDoneNotifier) Done() <-chan struct{} {
	doneCalled := c.doneCalled.Swap(nil)
	if doneCalled != nil {
		close(*doneCalled)
	}
	return c.Context.Done()
}
