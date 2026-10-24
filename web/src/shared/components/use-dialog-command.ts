// Source: docklet-next/issues/205
// Revision: be813fd8a9f61eaf99161d4141e5c324b5c34094
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original shared inline rejection state and accessible modal error view.

import { isCancelledError } from "@tanstack/react-query";
import { useLayoutEffect, useState } from "react";
import { toast } from "sonner";
import { useCommandError } from "@/data/use-command-error";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { withCommandFailure, type CommandFailure, type FailureOptions } from "./command-failure";

type Failure = { message: string; options?: FailureOptions } | null;
function controller() {
  let state: Failure = null;
  let mounted = false;
  let owner: unknown;
  let session = { active: false, closed: false };
  let operation: { failed: boolean } | undefined;
  const listeners = new Set<() => void>();
  const publish = (next: Failure) => { state = next; listeners.forEach((notify) => notify()); };
  // Editing clears presentation, never ownership or settlement of a pending attempt.
  const clear = () => { if (state) publish(null); };
  return {
    subscribe: (notify: () => void) => { listeners.add(notify); return () => { listeners.delete(notify); }; },
    snapshot: () => state,
    mount: () => { mounted = true; },
    unmount: () => { mounted = false; session.active = false; },
    activate: (active: boolean, nextOwner: unknown) => {
      if (owner !== nextOwner) { session.active = false; owner = nextOwner; session = { active, closed: false }; publish(null); }
      else if (active && !session.active) { session = { active: true, closed: false }; publish(null); }
      else if (!active && session.active) { session.active = false; session.closed = true; publish(null); }
    },
    dismiss: () => { session.closed = true; session.active = false; publish(null); },
    clear,
    checkpoint: () => {
      const started = session;
      return () => mounted && session === started && started.active;
    },
    failed: () => operation?.failed ?? false,
    invoke<T>(fn: () => T, adapter: ReturnType<typeof useAdapter>, errorText: (error: unknown) => string): T | undefined {
      clear();
      const attempt = { failed: false };
      operation = attempt;
      const started = session;
      const startedOwner = owner;
      const lifetime = identityLifetime(adapter);
      const version = lifetime.version();
      const sameIdentity = () => lifetime.active() && lifetime.version() === version;
      const current = () => sameIdentity() && owner === startedOwner && mounted && session === started && started.active;
      const receipt: CommandFailure = {
        current,
        closed: () => sameIdentity() && started.closed,
        error: (message, options) => {
          if (!sameIdentity()) return;
          if (current()) { attempt.failed = true; publish({ message, options }); }
          else if (started.closed) toast.error(message, options);
        },
      };
      try {
        const result = withCommandFailure(receipt, fn);
        if (!result || typeof (result as unknown as Promise<unknown>).then !== "function") return result;
        return Promise.resolve(result).then((value) => current() ? value : undefined, (error: unknown) => {
          if (!isCancelledError(error)) receipt.error(errorText(error));
          return undefined;
        }) as T;
      } catch (error) {
        if (!isCancelledError(error)) receipt.error(errorText(error));
        return undefined;
      }
    },
    reject: (message: string) => { operation = { failed: true }; publish({ message }); },
  };
}

export function useDialogCommand(open = true, owner?: unknown, formatError?: (error: unknown) => string) {
  const [feedback] = useState(controller);
  const adapter = useAdapter();
  const errorText = useCommandError();
  useLayoutEffect(() => { feedback.mount(); return feedback.unmount; }, [feedback]);
  useLayoutEffect(() => { feedback.activate(open, owner); }, [feedback, open, owner]);
  return {
    ...feedback,
    checkpoint: () => {
      const current = feedback.checkpoint();
      const lifetime = identityLifetime(adapter), version = lifetime.version();
      return () => current() && lifetime.active() && lifetime.version() === version;
    },
    run: <T,>(fn: () => T) => feedback.invoke(fn, adapter, formatError ?? errorText),
    change: (next: boolean, notify: (open: boolean) => void) => { if (!next) feedback.dismiss(); notify(next); },
    bindings: {
      onChangeCapture: feedback.clear,
      onInputCapture: feedback.clear,
      onClickCapture: (event: React.MouseEvent<HTMLElement>) => {
        const target = event.target as HTMLElement;
        if (target.closest('[disabled], [aria-busy="true"]')) return;
        if (target.closest('button, [role="checkbox"], [role="switch"], [role="radio"], [role="option"], [role="combobox"]')) feedback.clear();
      },
    },
  };
}
export type DialogCommand = ReturnType<typeof useDialogCommand>;

