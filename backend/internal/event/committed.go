// Source: docklet-next/issues/335
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original collaborating-domain event invalidation seam.

package event

// NotifyCommitted invalidates event readers after a collaborating domain commits events.
func (s *EventService) NotifyCommitted() { s.changes.Publish(struct{}{}) }
