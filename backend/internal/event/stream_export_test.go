// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original test-only subscription barrier for the real event stream producer.

package event

// SetSubscriptionBarrierForTest pauses immediately before the real signal subscription.
func (s *EventService) SetSubscriptionBarrierForTest(before func()) {
	s.changes = subscriptionBarrier{signal: s.changes, before: before}
}

type subscriptionBarrier struct {
	signal interface {
		Subscribe(func(struct{})) func()
		Publish(struct{})
	}
	before func()
}

func (b subscriptionBarrier) Subscribe(f func(struct{})) func() {
	b.before()
	return b.signal.Subscribe(f)
}
func (b subscriptionBarrier) Publish(v struct{}) { b.signal.Publish(v) }
