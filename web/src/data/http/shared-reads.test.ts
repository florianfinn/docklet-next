// Source: docklet-next/issues/271
// Revision: 4e71e45a2297a4f2719188cdf15c3e4d586b5357
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original shared inventory cancellation and body ownership regressions.

import { expect, it, vi } from "vitest";
import { createSharedReads } from "./shared-reads";
import { deferred } from "./activity-reader.test-support";
const path = "/api/environments/example/containers?start=0";
it("one cancelled reader leaves the shared request alive for the other query", async () => {
  const gate = deferred<Response>(); let physical!: AbortSignal;
  const fetch = vi.fn(async (_input: RequestInfo | URL, init?: RequestInit) => { physical = init!.signal!; return gate.promise; });
  const reads = createSharedReads(fetch), first = new AbortController(), second = new AbortController();
  const a = reads.fetch(path, { signal: first.signal }).catch((error: unknown) => error);
  const b = reads.fetch(path, { signal: second.signal });
  first.abort(); expect(await a).toBe(first.signal.reason); expect(physical.aborted).toBe(false);
  gate.resolve(Response.json({ value: "fresh" })); expect(await (await b).json()).toEqual({ value: "fresh" });
  expect(fetch).toHaveBeenCalledTimes(1); expect(physical.aborted).toBe(false);
});
it("the last cancelling reader aborts the physical request and late headers remain ignored", async () => {
  const gate = deferred<Response>(); let physical!: AbortSignal;
  const reads = createSharedReads(async (_input, init) => { physical = init!.signal!; return gate.promise; });
  const first = new AbortController(), second = new AbortController();
  const a = reads.fetch(path, { signal: first.signal }).catch((error: unknown) => error);
  const b = reads.fetch(path, { signal: second.signal }).catch((error: unknown) => error);
  first.abort(); expect(physical.aborted).toBe(false); second.abort();
  expect(await a).toBe(first.signal.reason); expect(await b).toBe(second.signal.reason); expect(physical.aborted).toBe(true);
  gate.resolve(Response.json({ value: "obsolete" })); expect(await b).toBe(second.signal.reason);
});
it("body readers retain independent cancellation ownership after headers arrive", async () => {
  let body!: ReadableStreamDefaultController<Uint8Array>; let physical!: AbortSignal;
  const reads = createSharedReads(async (_input, init) => { physical = init!.signal!; return new Response(new ReadableStream({ start(controller) { body = controller; } })); });
  const first = new AbortController(), second = new AbortController();
  const [a, b] = await Promise.all([reads.fetch(path, { signal: first.signal }), reads.fetch(path, { signal: second.signal })]);
  const rejected = a.text().catch((error: unknown) => error); const healthy = b.text();
  first.abort(); expect(await rejected).toBe(first.signal.reason); expect(physical.aborted).toBe(false);
  body.enqueue(new TextEncoder().encode("healthy")); body.close(); expect(await healthy).toBe("healthy");
  second.abort(); expect(physical.aborted).toBe(false);
});
