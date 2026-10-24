// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original dialog-owned command failure receipts.

import { toast } from "sonner";
import type { DockletAdapter } from "@/data/adapter";
import { identityLifetime } from "@/data/identity-lifetime";

export interface FailureOptions {
  action?: { label: string; onClick: () => void };
}
export interface CommandFailure {
  error: (message: string, options?: FailureOptions) => void;
  current: () => boolean;
  closed: () => boolean;
}
let target: CommandFailure | undefined;

/** Only synchronous invocation is scoped; asynchronous settlement owns its captured receipt. */
export function withCommandFailure<T>(receipt: CommandFailure, invoke: () => T): T {
  const previous = target;
  target = receipt;
  try { return invoke(); } finally { target = previous; }
}

export function captureCommandFailure(adapter: DockletAdapter): CommandFailure {
  const receipt = target;
  const lifetime = identityLifetime(adapter);
  const version = lifetime.version();
  const current = () => lifetime.active() && lifetime.version() === version;
  return {
    error: (message, options) => {
      if (!current()) return;
      if (receipt) receipt.error(message, options);
      else if (options) toast.error(message, options);
      else toast.error(message);
    },
    current: () => current() && (receipt?.current() ?? true),
    closed: () => current() && (receipt?.closed() ?? false),
  };
}
