// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import type { RuntimeIdentityBlocker } from "./runtime-blocker";

export interface RuntimePeerConfirmation {
 blocker: RuntimeIdentityBlocker;
 host: string;
 signal: AbortSignal;
 settle: (confirmed: boolean) => void;
}
let consumer: ((request: RuntimePeerConfirmation) => void) | undefined;
export function registerRuntimePeerConfirmation(next: (request: RuntimePeerConfirmation) => void) {
 consumer = next;
 return () => { if (consumer === next) consumer = undefined; };
}

// The action owner awaits a deliberate answer only after a definitive refusal.
// Missing UI or identity cancellation fails closed; neither can resend an action.
export function confirmRuntimePeers(blocker: RuntimeIdentityBlocker, host: string, signal: AbortSignal): Promise<boolean> {
 if (!consumer || signal.aborted) return Promise.resolve(false);
 return new Promise((resolve) => {
  let done = false;
  const settle = (value: boolean) => { if (done) return; done = true; signal.removeEventListener("abort", abort); resolve(value); };
  const abort = () => settle(false);
  signal.addEventListener("abort", abort, { once: true });
  consumer!({ blocker, host, signal, settle });
 });
}
