// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original identity-scoped event history coalescer using the shared single-flight state machine.

import { createSingleFlight } from "./single-flight";

/** Complete histories use the same notification and generation policy as discovery. */
export function createEventReader<T>(read: () => Promise<T>, generation: () => number, hasConsumer: () => boolean = () => false) {
  const flight = createSingleFlight(read, generation, { hasConsumer });
  return { load: flight.request, identityChanged: flight.identityChanged, changed() { void flight.notify().catch(() => undefined); } };
}
