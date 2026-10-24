// Source: docklet-next/issues/211
// Revision: 79ee005c9dbf2fb261bd413a6d273c4cce71c2e3
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original identity-scoped event history coalescer with one trailing read.

import { CancelledError } from "@tanstack/react-query";

/** Each group has one current read and at most one trailing complete history. */
export function createEventReader<T>(read: () => Promise<T>, generation: () => number) {
  let pending: { generation: number; dirty: boolean; promise: Promise<T> } | undefined;
  function load(): Promise<T> {
    if (pending?.generation === generation()) return pending.promise;
    const work = { generation: generation(), dirty: false, promise: undefined as unknown as Promise<T> };
    const check = () => { if (work.generation !== generation()) throw new CancelledError(); };
    work.promise = (async () => {
      let result = await read();
      check();
      if (work.dirty) {
        work.dirty = false;
        result = await read();
        check();
      }
      return result;
    })();
    pending = work;
    const clear = () => { if (pending === work) pending = undefined; };
    void work.promise.then(clear, clear);
    return work.promise;
  }
  return { load, changed() { if (pending?.generation === generation()) pending.dirty = true; } };
}
