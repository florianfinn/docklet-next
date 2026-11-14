// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original fair physical admission, cancellation, protocol and response lifetime proofs. M26 #462: read-only POST summaries share the physical read budget.

import { afterEach, expect, it, vi } from "vitest";
import { createHostReadAdmission } from "./host-read-admission";
import { createSharedReads } from "./shared-reads";
import { deferred } from "./activity-reader.test-support";

const path = (id: number, host = "direct") => `/api/environments/${host}/containers/${id}`;
const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };
const fill = (fetch: typeof globalThis.fetch) => Promise.all([2, 3, 4, 5].map((i) => fetch(path(i))));
const consume = (responses: Response[]) => Promise.all(responses.map((r) => r.text()));
afterEach(() => { vi.useRealTimers(); vi.restoreAllMocks(); });

it("C1-1a a dense lone HTTP/1.1 host starts five of eight reads immediately", async () => {
  const physical = vi.fn(async () => Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical, "/api", () => "http/1.1");
  const reads = Array.from({ length: 8 }, (_, i) => fetch(path(i)));
  await flush(); expect(physical.mock.calls.length).toBe(5);
  await (await reads[0]!).text(); await flush(); expect(physical.mock.calls.length).toBe(6);
  await consume(await Promise.all(reads.slice(1, 5)));
  await consume(await Promise.all(reads.slice(5)));
});

it.each(["h2", "h3"])("C1-1b %s starts all twelve reads without admission", async (protocol) => {
  const physical = vi.fn(async () => Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical, "/api", () => protocol);
  const reads = Array.from({ length: 12 }, (_, i) => fetch(path(i)));
  await flush(); expect(physical.mock.calls.length).toBe(12);
  await consume(await Promise.all(reads));
});

it.each(["http/1.1", "", "unavailable"])("uses navigation protocol %s with a conservative unavailable fallback", async (protocol) => {
  vi.spyOn(performance, "getEntriesByType").mockReturnValue(protocol === "unavailable" ? [] : [{ nextHopProtocol: protocol } as PerformanceNavigationTiming]);
  const physical = vi.fn(async () => Response.json({ ok: true })); const fetch = createHostReadAdmission(physical);
  const reads = Array.from({ length: 6 }, (_, i) => fetch(path(i)));
  await flush(); expect(physical.mock.calls.length).toBe(5);
  await (await reads[0]!).text(); await consume(await Promise.all(reads.slice(1)));
});

it("reads h2 from navigation timing by default", async () => {
  vi.spyOn(performance, "getEntriesByType").mockReturnValue([{ nextHopProtocol: "h2" } as PerformanceNavigationTiming]);
  const physical = vi.fn(async () => Response.json({ ok: true })); const fetch = createHostReadAdmission(physical);
  const reads = Array.from({ length: 12 }, (_, i) => fetch(path(i)));
  await flush(); expect(physical.mock.calls.length).toBe(12); await consume(await Promise.all(reads));
});

it("D4-2 removes an aborted queued GET immediately and preserves FIFO for survivors", async () => {
  const sent: string[] = [];
  const physical = vi.fn(async (input: RequestInfo | URL) => { sent.push(String(input)); return Response.json({ ok: true }); });
  const fetch = createHostReadAdmission(physical);
  const held = await Promise.all([1, 2, 3, 4, 5].map((i) => fetch(path(i))));
  const controller = new AbortController();
  let stopped = false;
  const cancelled = fetch(path(6), { signal: controller.signal }).catch((error: unknown) => { stopped = true; return error; });
  const seventh = fetch(path(7)), eighth = fetch(path(8));
  controller.abort(); await flush(); expect(stopped).toBe(true); expect(await cancelled).toBe(controller.signal.reason);
  expect(physical.mock.calls.length).toBe(5);
  await held[0]!.text(); const next = await seventh;
  expect(physical.mock.calls.length).toBe(6);
  await next.text(); await (await eighth).text(); await consume(held.slice(1));
  expect(sent).toEqual([1, 2, 3, 4, 5, 7, 8].map((i) => path(i)));
});

it("chooses the least-active queued host, then oldest wait, with FIFO within each host", async () => {
  const sent: string[] = [];
  const fetch = createHostReadAdmission(vi.fn(async (input) => { sent.push(String(input)); return Response.json({ ok: true }); }));
  const held = await Promise.all([1, 2, 3, 4, 5].map((i) => fetch(path(i))));
  const direct = fetch(path(6)), edge = fetch(path(1, "edge")), local = fetch(path(1, "0")), edge2 = fetch(path(2, "edge"));
  await held[0]!.text(); await flush(); expect(sent.at(-1)).toBe(path(1, "edge"));
  await held[1]!.text(); await flush(); expect(sent.at(-1)).toBe(path(1, "0"));
  await held[2]!.text(); await flush(); expect(sent.at(-1)).toBe(path(2, "edge"));
  await consume(held.slice(3)); await consume(await Promise.all([direct, edge, local, edge2]));
});

it("C1-1c restrains a held host after its first long unanswered abort and clears at headers", async () => {
  vi.useFakeTimers();
  const gate = deferred<Response>(), controller = new AbortController();
  const sent: string[] = [];
  const fetch = createHostReadAdmission(vi.fn(async (input) => { sent.push(String(input)); return String(input) === path(1) ? gate.promise : Response.json({ ok: true }); }));
  const first = fetch(path(1), { signal: controller.signal });
  const held = await fill(fetch);
  const direct = fetch(path(6)), edge = fetch(path(1, "edge")), local = fetch(path(1, "0"));
  await vi.advanceTimersByTimeAsync(10_000); controller.abort(); await flush();
  expect(sent.includes(path(6))).toBe(false);
  expect(sent.at(-1)).toBe(path(1, "edge"));
  await consume(held.slice(0, 3)); await flush();
  expect(sent.includes(path(6))).toBe(false); // One surviving Direct read still occupies its restraint.
  expect(sent.includes(path(1, "0"))).toBe(true);
  const direct2 = fetch(path(7));
  gate.resolve(Response.json({ late: true })); await flush();
  // Headers clear restraint even while the old body and a surviving Direct body remain unread.
  expect(sent.includes(path(6))).toBe(true); expect(sent.includes(path(7))).toBe(true);
  await (await first).text(); await held[3]!.text(); await consume(await Promise.all([direct, direct2, edge, local]));
});

it("C1-1c retains the held mark across an idle batch, reserving one Direct and four healthy slots until headers", async () => {
  vi.useFakeTimers(); const controller = new AbortController(), headers = deferred<Response>();
  const sent: string[] = [];
  const fetch = createHostReadAdmission(vi.fn((input, init) => {
    const url = String(input); sent.push(url);
    if (init?.signal === controller.signal) return new Promise<Response>((_resolve, reject) => controller.signal.addEventListener("abort", () => reject(controller.signal.reason), { once: true }));
    return url.startsWith("/api/environments/direct/") ? headers.promise.then((response) => response.clone()) : Promise.resolve(Response.json({ ok: true }));
  }));
  const batch = Array.from({ length: 5 }, (_, i) => fetch(path(i), { signal: controller.signal }).catch(() => undefined));
  await flush(); await vi.advanceTimersByTimeAsync(10_000); controller.abort(); await Promise.all(batch);
  const direct = [fetch(path(10)), fetch(path(11)), fetch(path(12))];
  const healthy = await Promise.all([fetch(path(1, "0")), fetch(path(2, "0")), fetch(path(1, "edge")), fetch(path(2, "edge"))]);
  expect(sent.filter((url) => url.includes("/direct/")).length).toBe(6);
  await healthy[0]!.text(); await flush();
  expect(sent.includes(path(11))).toBe(false);
  headers.resolve(Response.json({ recovered: true })); await flush();
  expect(sent.includes(path(11))).toBe(true);
  await (await direct[0]!).text();
  await consume(healthy.slice(1));
  await consume(await Promise.all(direct.slice(1)));
});

it.each(["short", "headers"])("does not mark a host held after %s abort", async (kind) => {
  vi.useFakeTimers();
  const gate = deferred<Response>(), controller = new AbortController();
  const physical = vi.fn(async (input) => String(input) === path(1) && kind === "short" ? gate.promise : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal }), held = await fill(fetch);
  const next = fetch(path(6)); await vi.advanceTimersByTimeAsync(kind === "short" ? 9_999 : 10_000);
  controller.abort(); await flush(); expect(physical.mock.calls.length).toBe(6);
  gate.resolve(Response.json({ late: true })); await (await first).text(); await (await next).text(); await consume(held);
});

it("D4-3 holds slots through headers and body chunks until the reader reaches the end", async () => {
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1)
    ? new Response(new ReadableStream({ start(controller) { body = controller; } })) : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const response = await fetch(path(1)), held = await fill(fetch);
  let started = false; const pending = fetch(path(6)).then((value) => { started = true; return value; });
  await flush(); expect(started).toBe(false);
  const reader = response.body!.getReader(); body.enqueue(new TextEncoder().encode("held"));
  expect((await reader.read()).done).toBe(false); await flush(); expect(physical.mock.calls.length).toBe(5);
  body.close(); expect((await reader.read()).done).toBe(true);
  await flush(); expect(started).toBe(true); await (await pending).text(); await consume(held);
});

it.each(["consume", "cancel", "error", "abort", "request-failure"] as const)("D4-3 releases a slot on %s", async (mode) => {
  const gate = deferred<Response>(); let body!: ReadableStreamDefaultController<Uint8Array>;
  const controller = new AbortController();
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1)
    ? mode === "request-failure" ? gate.promise : new Response(new ReadableStream({ start(value) { body = value; } }))
    : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal }); const failed = first.catch((error: unknown) => error);
  const held = await fill(fetch);
  let started = false; const next = fetch(path(6)).then((value) => { started = true; return value; });
  await flush(); expect(started).toBe(false);
  if (mode === "request-failure") { gate.reject(new Error("failed")); expect(await failed).toBeInstanceOf(Error); }
  else {
    const response = await first;
    if (mode === "consume") { const reading = response.clone().text(); body.enqueue(new TextEncoder().encode("ok")); body.close(); expect(await reading).toBe("ok"); }
    if (mode === "cancel") await response.body!.cancel();
    if (mode === "error") { const reading = response.text().catch((error: unknown) => error); body.error(new Error("lost body")); expect(await reading).toBeInstanceOf(Error); }
    if (mode === "abort") controller.abort();
  }
  await flush(); expect(started).toBe(true); await (await next).text(); await consume(held);
});

it("bypasses writes, hub reads, downloads and SSE while admitted reads are full", async () => {
  const physical = vi.fn(async () => Response.json({ ok: true })); const fetch = createHostReadAdmission(physical);
  const held = await Promise.all([1, 2, 3, 4, 5].map((i) => fetch(path(i))));
  const pending = fetch(path(6));
  const others = await Promise.all([
    fetch(path(7), { method: "POST" }), fetch("/api/environments"), fetch("/api/stream"),
    fetch(path(8), { headers: { Accept: "text/event-stream" } }),
    fetch("/api/environments/direct/backups/r1/download"), fetch("/api/environments/direct/volumes/v1/file/download?relativePath=a"),
  ]);
  expect(physical.mock.calls.length).toBe(11);
  await consume(held); await (await pending).text(); await consume(others);
});

it("joined shared inventory reads occupy one physical slot", async () => {
  const physical = vi.fn(async () => Response.json({ ok: true }));
  const reads = createSharedReads(createHostReadAdmission(physical));
  const url = "/api/environments/direct/containers?start=0";
  const [a, b] = await Promise.all([reads.fetch(url), reads.fetch(url)]);
  const c = await reads.fetch(path(2));
  expect(physical.mock.calls.length).toBe(2);
  expect(await a.json()).toEqual({ ok: true }); expect(await b.json()).toEqual({ ok: true }); await c.text();
});

it("releases a body that errors before the caller reads it once the read settles", async () => {
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1)
    ? new Response(new ReadableStream({ start(controller) { body = controller; } })) : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = await fetch(path(1)), held = await fill(fetch);
  let started = false; const next = fetch(path(6)).then((response) => { started = true; return response; });
  body.error(new Error("lost unread body")); await flush(); expect(started).toBe(false);
  await expect(first.text()).rejects.toThrow("lost unread body"); await flush(); expect(started).toBe(true);
  await consume(held); await (await next).text();
});

// Chromium records a completed no-store body drained by a JS stream reader as aborted (#326).
it("reads method-consumed bodies natively without a stream reader on the physical response", async () => {
  const responses: Response[] = [];
  const physical = vi.fn(async () => { const response = Response.json({ ok: true }); responses.push(response); vi.spyOn(response.body!, "getReader"); return response; });
  const fetch = createHostReadAdmission(physical);
  expect(await (await fetch(path(1))).json()).toEqual({ ok: true });
  const cloned = (await fetch(path(2))).clone(); expect(await cloned.text()).toBe('{"ok":true}');
  expect(responses.map((response) => vi.mocked(response.body!.getReader).mock.calls.length)).toEqual([0, 0]);
});

it("aborting an unanswered request releases its slot even when the transport ignores abort", async () => {
  const gate = deferred<Response>(), controller = new AbortController();
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1) ? gate.promise : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal }), held = await fill(fetch);
  let started = false; const next = fetch(path(6)).then((response) => { started = true; return response; });
  await flush(); expect(started).toBe(false); controller.abort(); await flush(); expect(started).toBe(true);
  gate.resolve(Response.json({ late: true })); await (await first).text(); await (await next).text(); await consume(held);
});

// #370: a hub-owned restart read takes the next free slot instead of queueing behind its host's
// agent-backed reads, and a slow host read never holds it.
it.each(["restarts/latest", "restarts/example-run"])("admits the hub-owned %s read ahead of queued host reads", async (route) => {
  const sent: string[] = [];
  const gates = new Map<string, ReturnType<typeof deferred<Response>>>();
  const physical = vi.fn(async (input: RequestInfo | URL) => {
    const url = String(input); sent.push(url);
    const gate = deferred<Response>(); gates.set(url, gate); return gate.promise;
  });
  const fetch = createHostReadAdmission(physical, "/api", () => "http/1.1");
  const host = Array.from({ length: 8 }, (_, i) => fetch(path(i)));
  await flush(); expect(sent.length).toBe(5);
  const restart = fetch(`/api/environments/direct/agent/${route}`);
  await flush(); expect(sent.length).toBe(5);
  gates.get(path(0))!.resolve(Response.json({ ok: true })); await (await host[0]!).text(); await flush();
  expect(sent.at(-1)).toBe(`/api/environments/direct/agent/${route}`);
  gates.get(`/api/environments/direct/agent/${route}`)!.resolve(Response.json({ ok: true })); await (await restart).text();
  // Every later host read is answered and consumed as soon as admission sends it.
  const rest = host.slice(1).map(async (read) => (await read).text());
  for (let round = 0; round < 20 && sent.length < 9; round++) { for (const gate of gates.values()) gate.resolve(Response.json({ ok: true })); await flush(); }
  for (const gate of gates.values()) gate.resolve(Response.json({ ok: true }));
  await Promise.all(rest);
});

it.each(["active-summaries", "summaries"])("admits read-only %s POSTs within the physical host read budget", async (suffix) => {
  const physical = vi.fn(async () => Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical, "/api", () => "http/1.1");
  const held = await Promise.all([1, 2, 3, 4, 5].map((i) => fetch(path(i))));
  const read = fetch(`/api/environments/direct/images/vulnerabilities/${suffix}`, { method: "POST", body: "{}" });
  await flush(); expect(physical.mock.calls.length).toBe(5);
  await held[0]!.text();
  await (await read).text(); await consume(held.slice(1));
  expect(physical.mock.calls.length).toBe(6);
});
