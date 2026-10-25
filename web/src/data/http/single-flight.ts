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

/** One running read and one pending slot; only a pre-error read may be interrupted. */
export function createSingleFlight<T>(read: (signal: AbortSignal) => Promise<T>, generation: () => number,
  { publish = () => undefined, hasConsumer = () => false, deadlineMs, lineageMs = 80_000, timeout = () => new Error("activity.read-timeout") }:
  { publish?: (value: T) => void; hasConsumer?: () => boolean; deadlineMs?: number; lineageMs?: number; timeout?: () => Error } = {}) {
  type Waiter = ReturnType<typeof waiter<T>>;
  type Work = { generation: number; current: Waiter; waiters: Waiter[]; next?: Waiter; explicitJoiner: boolean;
    controller: AbortController; order: number; protected: boolean; lineageEnd: number; retired: boolean; timer?: ReturnType<typeof setTimeout> };
  let running: Work | undefined;
  let order = 0;
  let errorMarker: number | undefined;
  function start(current: Waiter, version: number, waiters = [current], protectedRead = false, lineageEnd = Date.now() + lineageMs): Promise<T> {
    if (version !== generation()) { waiters.forEach((w) => w.reject(new CancelledError())); return current.promise; }
    const work: Work = { generation: version, current, waiters, explicitJoiner: false, controller: new AbortController(),
      order: ++order, protected: protectedRead, lineageEnd, retired: false };
    running = work; // Consume pending before accepting another notification.
    void (async () => {
      try {
        const task = read(work.controller.signal);
        const value = deadlineMs === undefined ? await task : await Promise.race([task, new Promise<never>((_, reject) => {
          work.timer = setTimeout(() => { const error = timeout(); work.controller.abort(error); reject(error); }, Math.max(0, Math.min(deadlineMs, lineageEnd - Date.now())));
        })]);
        if (version !== generation() || running !== work || work.controller.signal.aborted) throw new CancelledError();
        publish(value);
        if (!work.retired) work.waiters.forEach((w) => w.resolve(value));
      } catch (error) {
        if (!work.retired) work.waiters.forEach((w) => w.reject(version === generation() ? error : new CancelledError()));
      } finally {
        clearTimeout(work.timer);
        if (!work.retired) {
          if (running === work) running = undefined;
          if (work.next) {
            if (hasConsumer() || work.explicitJoiner) start(work.next, version);
            else work.next.resolve(undefined as T);
          }
        }
      }
    })();
    return current.promise;
  }
  function active() { if (running && running.generation !== generation()) cancel(); return running; }
  function retire(work: Work) { work.retired = true; clearTimeout(work.timer); work.controller.abort(); }
  function cancel(reason: unknown = new CancelledError()) {
    const work = running; running = undefined;
    if (!work) return;
    retire(work); work.waiters.forEach((w) => w.reject(reason)); work.next?.reject(reason); work.next = undefined;
  }
  function request(): Promise<T> {
    const work = active();
    if (!work) return start(waiter<T>(), generation());
    if (work.next) work.explicitJoiner = true;
    return (work.next ?? work.current).promise;
  }
  function notify(): Promise<T | undefined> {
    const work = active();
    if (!work) return hasConsumer() ? start(waiter<T>(), generation()) : Promise.resolve(undefined);
    work.next ??= waiter<T>();
    return work.next.promise;
  }
  function recover(): Promise<T | undefined> {
    const work = active();
    if (!work) return notify();
    if (work.protected) return work.current.promise;
    if (errorMarker === undefined || work.order >= errorMarker) return notify();
    work.next ??= waiter<T>();
    const next = work.next;
    if (work.retired) return next.promise;
    retire(work);
    queueMicrotask(() => {
      if (running !== work) return;
      const waiters = [...work.waiters, next]; work.waiters = []; work.next = undefined; running = undefined;
      start(next, work.generation, waiters, true, work.lineageEnd);
    });
    return next.promise;
  }
  return { request, notify, recover, cancel, isActive: () => active() !== undefined,
    absorbNotifications() {
      const work = active();
      if (!work?.next || work.explicitJoiner) return;
      const next = work.next; work.next = undefined;
      void work.current.promise.then(next.resolve, next.reject);
    },
    recoverActive() { const work = active(); if (work && errorMarker !== undefined && work.order < errorMarker && !work.protected) void recover().catch(() => undefined); },
    dirty() { if (active()) void notify().catch(() => undefined); }, markError: () => { errorMarker = ++order; }, identityChanged: () => { active(); } };
}
