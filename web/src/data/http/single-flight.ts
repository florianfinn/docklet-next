// Source: docklet-next/issues/211
// Revision: 00b0dacea1c004c60e92d1cf0a2025734a468188
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original consumer-aware, generation-scoped single flight with interruptible recovery and waiter transfer.

import { CancelledError } from "@tanstack/react-query";

function waiter<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((done, fail) => { resolve = done; reject = fail; });
  void promise.catch(() => undefined);
  return { promise, resolve, reject };
}

/** One running read and one pending slot; idle notifications require live demand. */
export function createSingleFlight<T>(read: (signal: AbortSignal) => Promise<T>, generation: () => number,
  { publish = () => undefined, hasConsumer = () => false }: { publish?: (value: T) => void; hasConsumer?: () => boolean } = {}) {
  type Waiter = ReturnType<typeof waiter<T>>;
  type Work = { generation: number; current: Waiter; waiters: Waiter[]; next?: Waiter; controller: AbortController };
  let running: Work | undefined;
  function start(current: Waiter, version: number, waiters = [current]): Promise<T> {
    if (version !== generation()) { waiters.forEach((w) => w.reject(new CancelledError())); return current.promise; }
    const work: Work = { generation: version, current, waiters, controller: new AbortController() };
    running = work; // Pending is cleared at START, including every trailing start.
    void (async () => {
      try {
        const value = await read(work.controller.signal);
        if (version !== generation() || running !== work || work.controller.signal.aborted) throw new CancelledError();
        publish(value);
        if (!work.controller.signal.aborted) work.waiters.forEach((w) => w.resolve(value));
      } catch (error) {
        if (!work.controller.signal.aborted) work.waiters.forEach((w) => w.reject(version === generation() ? error : new CancelledError()));
      } finally {
        if (!work.controller.signal.aborted) {
          if (running === work) running = undefined;
          if (work.next) start(work.next, version);
        }
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
    work?.controller.abort();
    work?.waiters.forEach((w) => w.reject(reason));
    work?.next?.reject(reason);
    if (work) work.next = undefined;
  }
  function request(): Promise<T> {
    const work = active();
    return work ? (work.next ?? work.current).promise : start(waiter<T>(), generation());
  }
  function notify(): Promise<T | undefined> {
    const work = active();
    // Idle loaders retain no cached result: the next request always reads fresh.
    if (!work) return hasConsumer() ? start(waiter<T>(), generation()) : Promise.resolve(undefined);
    work.next ??= waiter<T>();
    return work.next.promise;
  }
  function recover(): Promise<T | undefined> {
    const work = active();
    if (!work) return notify();
    work.next ??= waiter<T>();
    const next = work.next;
    if (work.controller.signal.aborted) return next.promise;
    // A burst shares one scheduled restart; later recovery may interrupt any new read.
    work.controller.abort();
    queueMicrotask(() => {
      if (running !== work) return;
      const waiters = [...work.waiters, next];
      work.waiters = []; work.next = undefined; running = undefined;
      start(next, work.generation, waiters);
    });
    return next.promise;
  }
  return { request, notify, recover, cancel, identityChanged: () => { active(); } };
}
