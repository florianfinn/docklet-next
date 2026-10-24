// Source: docklet-next/issues/211
// Revision: 00b0dacea1c004c60e92d1cf0a2025734a468188
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original generation-scoped single-flight state machine shared by full readers.

import { CancelledError } from "@tanstack/react-query";

function waiter<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  void promise.catch(() => undefined);
  return { promise, resolve, reject };
}

/** One running read and one pending slot; each start reopens that slot. */
export function createSingleFlight<T>(read: () => Promise<T>, generation: () => number, publish: (value: T) => void = () => undefined) {
  type Waiter = ReturnType<typeof waiter<T>>;
  type Work = { generation: number; current: Waiter; next?: Waiter };
  let running: Work | undefined;
  function start(current: Waiter, version: number): Promise<T> {
    // Check even when a completed old read is trying to start queued work.
    if (version !== generation()) { current.reject(new CancelledError()); return current.promise; }
    const work: Work = { generation: version, current };
    running = work; // Pending is cleared at START, including every trailing start.
    void (async () => {
      try {
        const value = await read();
        if (version !== generation() || running !== work) throw new CancelledError();
        publish(value);
        current.resolve(value);
      } catch (error) {
        current.reject(version === generation() ? error : new CancelledError());
      } finally {
        if (running === work) running = undefined;
        if (work.next) start(work.next, version);
      }
    })();
    return current.promise;
  }
  function active() {
    if (running && running.generation !== generation()) cancel();
    return running;
  }
  function cancel(reason: unknown = new CancelledError()) {
    const work = running;
    running = undefined;
    work?.current.reject(reason);
    work?.next?.reject(reason);
    if (work) work.next = undefined;
  }
  function request(): Promise<T> {
    const work = active();
    return work ? (work.next ?? work.current).promise : start(waiter<T>(), generation());
  }
  function notify(): Promise<T> {
    const work = active();
    if (!work) return start(waiter<T>(), generation());
    work.next ??= waiter<T>();
    return work.next.promise;
  }
  return { request, notify, cancel };
}
