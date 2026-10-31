import { useCallback, useLayoutEffect, useRef, useState } from "react";

import { useActionRun } from "@/data/live";
import type { ActionStatus } from "@/domain/model";

/** What a busy action returns: an action run id to follow, or any other settled value. */
type ActionResult = string | null | undefined | void | boolean | { status: "online" | "standby-opened" };

export type BusyState = "idle" | "calling" | ActionStatus;

/**
 * Busy state of one execution button. It stays busy while the adapter call is
 * in flight and, when the call returns an action run id, until that run
 * leaves "running". Further runs are ignored while busy; a thrown error ends
 * the busy state and is kept until the next run.
 */
export function useBusyAction<A extends unknown[]>(fn: (...args: A) => Promise<ActionResult>) {
  const fnRef = useRef(fn);
  useLayoutEffect(() => {
    fnRef.current = fn;
  });
  const [calling, setCalling] = useState(false);
  const [runId, setRunId] = useState<string | undefined>();
  const [error, setError] = useState<unknown>(null);
  const run = useActionRun(runId);
  const inFlight = useRef(false);
  const tracking = run?.status === "running";

  const start = useCallback(async (...args: A) => {
    if (inFlight.current) return undefined;
    inFlight.current = true;
    setCalling(true);
    setError(null);
    setRunId(undefined);
    try {
      const result = await fnRef.current(...args);
      if (typeof result === "string" && result) setRunId(result);
      return result;
    } catch (e) {
      setError(e);
      return undefined;
    } finally {
      inFlight.current = false;
      setCalling(false);
    }
  }, []);

  const state: BusyState = calling ? "calling" : (run?.status ?? (error ? "failed" : "idle"));
  return { busy: calling || tracking, state, error, run: start };
}
