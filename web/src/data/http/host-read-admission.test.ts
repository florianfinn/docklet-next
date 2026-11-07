// Source: docklet-next/issues/271
// Revision: bbab488e30d2e91edbd840bca929737c3ac1ab5c
// Retrieved: 2026-11-07
// License: Apache-2.0
// Deviations: Original physical admission, FIFO cancellation and response lifetime proofs.

import { expect, it, vi } from "vitest";
import { createHostReadAdmission } from "./host-read-admission";
import { createSharedReads } from "./shared-reads";
import { deferred } from "./activity-reader.test-support";

const path = (id: number, host = "direct") => `/api/environments/${host}/containers/${id}`;
const flush = async () => { for (let i = 0; i < 20; i++) await Promise.resolve(); };

it("D4-2 removes an aborted queued GET immediately and preserves FIFO for the survivors", async () => {
  const sent: string[] = [];
  const physical = vi.fn(async (input: RequestInfo | URL) => { sent.push(String(input)); return Response.json({ ok: true }); });
  const fetch = createHostReadAdmission(physical);
  const a = await fetch(path(1)), b = await fetch(path(2));
  const controller = new AbortController();
  let stopped = false;
  const cancelled = fetch(path(3), { signal: controller.signal }).catch((error: unknown) => { stopped = true; return error; });
  const fourth = fetch(path(4)), fifth = fetch(path(5));
  controller.abort(); await flush(); expect(stopped).toBe(true); expect(await cancelled).toBe(controller.signal.reason);
  expect(physical.mock.calls.length).toBe(2);
  await a.text(); const c = await fourth;
  expect(physical.mock.calls.length).toBe(3);
  await c.text(); await (await fifth).text(); await b.text();
  expect(sent).toEqual([path(1), path(2), path(4), path(5)]);
});

it("D4-3 holds slots through headers and body chunks until the reader reaches the end", async () => {
  let body!: ReadableStreamDefaultController<Uint8Array>;
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1)
    ? new Response(new ReadableStream({ start(controller) { body = controller; } })) : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const response = await fetch(path(1)); const second = await fetch(path(2));
  let started = false; const pending = fetch(path(3)).then((value) => { started = true; return value; });
  await flush(); expect(started).toBe(false);
  const reader = response.body!.getReader(); body.enqueue(new TextEncoder().encode("held"));
  expect((await reader.read()).done).toBe(false);
  await flush(); expect(physical.mock.calls.length).toBe(2);
  body.close(); expect((await reader.read()).done).toBe(true);
  await flush(); expect(started).toBe(true); await (await pending).text(); await second.text();
});

it.each(["consume", "cancel", "error", "abort", "request-failure"] as const)("D4-3 releases a slot on %s", async (mode) => {
  const gate = deferred<Response>(); let body!: ReadableStreamDefaultController<Uint8Array>;
  const controller = new AbortController();
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1)
    ? mode === "request-failure" ? gate.promise : new Response(new ReadableStream({ start(value) { body = value; } }))
    : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal }); const failed = first.catch((error: unknown) => error);
  const second = await fetch(path(2));
  let started = false; const third = fetch(path(3)).then((value) => { started = true; return value; });
  await flush(); expect(started).toBe(false);
  if (mode === "request-failure") { gate.reject(new Error("failed")); expect(await failed).toBeInstanceOf(Error); }
  else {
    const response = await first;
    if (mode === "consume") { const reading = response.clone().text(); body.enqueue(new TextEncoder().encode("ok")); body.close(); expect(await reading).toBe("ok"); }
    if (mode === "cancel") await response.body!.cancel();
    if (mode === "error") { const reading = response.text().catch((error: unknown) => error); body.error(new Error("lost body")); expect(await reading).toBeInstanceOf(Error); }
    if (mode === "abort") controller.abort();
  }
  await flush(); expect(started).toBe(true); await (await third).text(); await second.text();
});

it("keeps two slots per host and bypasses writes, hub reads and SSE", async () => {
  const physical = vi.fn(async () => Response.json({ ok: true })); const fetch = createHostReadAdmission(physical);
  const a = await fetch(path(1)), b = await fetch(path(2));
  const pending = fetch(path(3));
  const others = await Promise.all([
    fetch(path(1, "edge")), fetch(path(2, "edge")), fetch(path(3), { method: "POST" }),
    fetch("/api/environments"), fetch("/api/stream"), fetch(path(4), { headers: { Accept: "text/event-stream" } }),
  ]);
  expect(physical.mock.calls.length).toBe(8);
  await a.text(); await (await pending).text(); await b.text(); await Promise.all(others.map((r) => r.text()));
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
  const first = await fetch(path(1)), second = await fetch(path(2));
  let started = false; const third = fetch(path(3)).then((response) => { started = true; return response; });
  body.error(new Error("lost unread body")); await flush(); expect(started).toBe(false);
  await expect(first.text()).rejects.toThrow("lost unread body"); await flush(); expect(started).toBe(true);
  await second.text(); await (await third).text();
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

it("does not admit downloads, so a transfer never holds a host slot", async () => {
  const physical = vi.fn(async () => Response.json({ ok: true })); const fetch = createHostReadAdmission(physical);
  const held = [await fetch(path(1)), await fetch(path(2))];
  const downloads = [fetch("/api/environments/direct/backups/r1/download"), fetch("/api/environments/direct/volumes/v1/file/download?relativePath=a")];
  await flush(); expect(physical.mock.calls.length).toBe(4);
  await Promise.all(held.map((response) => response.text())); await Promise.all((await Promise.all(downloads)).map((response) => response.text()));
});

it("aborting an unanswered request releases its slot even when the transport ignores abort", async () => {
  const gate = deferred<Response>(), controller = new AbortController();
  const physical = vi.fn(async (input: RequestInfo | URL) => String(input) === path(1) ? gate.promise : Response.json({ ok: true }));
  const fetch = createHostReadAdmission(physical);
  const first = fetch(path(1), { signal: controller.signal }), second = await fetch(path(2));
  let started = false; const third = fetch(path(3)).then((response) => { started = true; return response; });
  await flush(); expect(started).toBe(false); controller.abort(); await flush(); expect(started).toBe(true);
  gate.resolve(Response.json({ late: true })); await (await first).text(); await (await third).text(); await second.text();
});
