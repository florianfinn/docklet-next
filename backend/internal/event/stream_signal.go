// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original event subscription and publication boundary.

package event

type eventChangeSignal interface {
	Subscribe(func(struct{})) func()
	Publish(struct{})
}
