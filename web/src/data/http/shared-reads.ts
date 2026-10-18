// Source: docklet-next/issues/100
// Revision: fdb37e836f708bd9ff76f75075bbb3e60ae2f0b3
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original adapter-local sharing of concurrent read responses.

/** Only concurrent GETs share work; each caller owns its response body. */
export function createSharedReads(fetcher: typeof fetch) {
  const pending = new Map<string, Promise<Response>>();
  const fetch: typeof globalThis.fetch = async (input, init) => {
    const method = init?.method ?? (input instanceof Request ? input.method : "GET");
    if (method !== "GET" || input instanceof Request || init?.signal) return fetcher(input, init);
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
  return { fetch, clear: () => pending.clear() };
}
