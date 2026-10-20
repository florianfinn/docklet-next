// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original session-bound viewer and browser identity lifecycle implementation.

import type { DockletAdapter } from "./adapter";

export interface IdentityLifetime {
  version: () => number;
  active: () => boolean;
  listen: (notify: () => void) => () => void;
}
const lifetimes = new WeakMap<object, IdentityLifetime>();
const idle: IdentityLifetime = { version: () => 0, active: () => true, listen: () => () => {} };
// The subscription function survives the RunGate proxy unchanged.
export function registerIdentityLifetime(subscribe: object, lifetime: IdentityLifetime) { lifetimes.set(subscribe, lifetime); }
export function identityLifetime(adapter: DockletAdapter) { return lifetimes.get(adapter.subscribeChanges) ?? idle; }
