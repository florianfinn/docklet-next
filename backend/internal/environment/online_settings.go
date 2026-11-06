// Source: docklet-next/issues/355
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original authenticated agent-online settings resend.

package environment

import (
	"context"
	"sync"
)

type onlineListeners struct {
	mu        sync.Mutex
	next      uint64
	callbacks map[uint64]func(context.Context, string)
}

func (s *EnvironmentService) SubscribeAgentOnline(fn func(context.Context, string)) func() {
	w := &s.onlineListeners
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.callbacks == nil {
		w.callbacks = map[uint64]func(context.Context, string){}
	}
	w.next++
	id := w.next
	w.callbacks[id] = fn
	return func() { w.mu.Lock(); defer w.mu.Unlock(); delete(w.callbacks, id) }
}
func (s *EnvironmentService) notifyAgentOnline(ctx context.Context, id string) {
	w := &s.onlineListeners
	w.mu.Lock()
	callbacks := make([]func(context.Context, string), 0, len(w.callbacks))
	for _, fn := range w.callbacks {
		callbacks = append(callbacks, fn)
	}
	w.mu.Unlock()
	for _, fn := range callbacks {
		fn(ctx, id)
	}
}
