// Source: docklet-next/issues/100
// Revision: fdb37e836f708bd9ff76f75075bbb3e60ae2f0b3
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original adapter-local sharing of concurrent read responses. M20 #360: shell read budgets and target-specific terminal invalidation.

// Only these inventory readers share abortable requests; other owned reads retain their own transport.
const INVENTORY = /\/environments\/[^/]+\/(?:projects|containers|images|volumes|networks|runtime-conditions)(?:\?|$)/;
interface SharedInventory {
  response: Promise<Response>;
  controller: AbortController;
  readers: Set<symbol>;
  drained: boolean;
}

/** Concurrent inventory readers own separate body leases and independent cancellation. */
function inventorySharing(fetcher: typeof fetch) {
  const pending = new Map<string, SharedInventory>();
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const signal = init?.signal;
    signal?.throwIfAborted();
    const key = JSON.stringify([String(input), init?.credentials, [...new Headers(init?.headers)]]);
    let entry = pending.get(key);
    if (!entry) {
      const controller = new AbortController();
      entry = { controller, readers: new Set(), drained: false, response: fetcher(input, { ...init, signal: controller.signal }) };
      pending.set(key, entry);
      const owner = entry;
      const remove = () => { if (pending.get(key) === owner) pending.delete(key); };
      void entry.response.then(remove, remove);
    }
    const owner = entry, reader = Symbol(); owner.readers.add(reader);
    let reject!: (error: unknown) => void;
    const stopped = new Promise<never>((_, fail) => { reject = fail; });
    const release = (cancelled = false) => {
      if (!owner.readers.delete(reader)) return;
      signal?.removeEventListener("abort", abort);
      if (cancelled && !owner.readers.size && !owner.drained) {
        if (pending.get(key) === owner) pending.delete(key);
        owner.controller.abort(signal?.reason);
      }
    };
    const abort = () => { reject(signal?.reason); release(true); };
    signal?.addEventListener("abort", abort, { once: true });
    if (signal?.aborted) abort();
    try {
      const response = (await Promise.race([owner.response, stopped])).clone();
      signal?.throwIfAborted();
      return new Proxy(response, {
        get(target, prop) {
          const member: unknown = Reflect.get(target, prop, target);
          if (["json", "text", "arrayBuffer", "blob", "formData", "bytes"].includes(String(prop)) && typeof member === "function") return async () => {
            try {
              signal?.throwIfAborted();
              const body = Promise.resolve(member.call(target)).then((value: unknown) => { owner.drained = true; signal?.throwIfAborted(); return value; });
              return await Promise.race([body, stopped]);
            } finally { release(); }
          };
          if (prop === "body" && member instanceof ReadableStream) return new Proxy(member, {
            get(stream, field) {
              if (field === "cancel") return (reason?: unknown) => { release(true); return stream.cancel(reason); };
              const value: unknown = Reflect.get(stream, field, stream);
              return typeof value === "function" ? value.bind(stream) : value;
            },
          });
          return typeof member === "function" ? member.bind(target) : member;
        },
      });
    } catch (error) { release(true); throw error; }
  };
  return { fetch, clear: () => pending.clear() };
}

/** Only concurrent GETs share work; each caller owns its response body. */
export function createSharedReads(fetcher: typeof fetch) {
  const inventory = inventorySharing(fetcher);
  const pending = new Map<string, Promise<Response>>();
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const method = init?.method ?? (input instanceof Request ? input.method : "GET");
    if (method !== "GET" || input instanceof Request) return fetcher(input, init);
    if (INVENTORY.test(String(input))) return inventory.fetch(input, init);
    if (init?.signal) return fetcher(input, init);
    const key = JSON.stringify([String(input), init?.credentials, [...new Headers(init?.headers)]]);
    let response = pending.get(key);
    if (!response) {
      const started = fetcher(input, init);
      pending.set(key, started);
      response = started;
      // Do not let an older read remove a replacement after invalidation.
      void started.then(() => { if (pending.get(key) === started) pending.delete(key); }, () => { if (pending.get(key) === started) pending.delete(key); });
    }
    return (await response).clone();
  };
  return { fetch, clear() { pending.clear(); inventory.clear(); } };
}
