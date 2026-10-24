// Source: docklet-next/issues/211
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original safe GET retry and cancellation regression tests.

import { expect, it, vi } from "vitest";
import { createIdentityReads } from "./identity-reads";
import { activityFixture, deferred } from "./activity-reader.test-support";

it.each([new TypeError("Failed to fetch"), new DOMException("Network failure", "NetworkError")])("retries a transport failure once and delivers the GET data (%s)", async (error) => {
  const fetcher = vi.fn().mockRejectedValueOnce(error).mockResolvedValueOnce(new Response('{"value":42}'));
  const reads = createIdentityReads(fetcher);
  expect(await (await reads.fetch("/api/events")).json()).toEqual({ value: 42 });
  expect(fetcher).toHaveBeenCalledTimes(2);
  expect(fetcher.mock.calls[0][1].signal).toBe(fetcher.mock.calls[1][1].signal);
});

it("keeps the existing adapter error path after two transport failures", async () => {
  const f = activityFixture();
  f.state.failure = "network";
  await f.adapter.listActions();
  expect(f.requests.filter((url) => url.startsWith("/api/environments/bad/activities?"))).toHaveLength(2);
  let health: unknown;
  f.adapter.subscribeActionReadHealth!((value) => { health = value; })();
  expect(health).toEqual({ failures: [{ hostId: "bad", reason: "unavailable" }] });
});

it.each(["POST", "PUT", "PATCH", "DELETE", "HEAD", "OPTIONS"])("never retries %s after a transport failure", async (method) => {
  const error = new TypeError("Failed to fetch");
  const fetcher = vi.fn().mockRejectedValue(error);
  const reads = createIdentityReads(fetcher);
  await expect(reads.fetch("/api/resource", { method })).rejects.toBe(error);
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("honours the method on Request inputs", async () => {
  const fetcher = vi.fn().mockRejectedValue(new TypeError("Failed to fetch"));
  await expect(createIdentityReads(fetcher).fetch(new Request("https://hub.example.com/api/resource", { method: "POST" }))).rejects.toBeInstanceOf(TypeError);
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it("does not retry HTTP 500 or a body failure after headers arrived", async () => {
  const response = new Response("invalid", { status: 500 });
  const fetcher = vi.fn().mockResolvedValue(response);
  const received = await createIdentityReads(fetcher).fetch("/api/events");
  expect(received.status).toBe(500);
  await expect(received.json()).rejects.toBeDefined();
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it.each(["AbortError", "Error"])("does not retry %s rejections", async (name) => {
  const error = new DOMException("Request stopped", name);
  const fetcher = vi.fn().mockRejectedValue(error);
  await expect(createIdentityReads(fetcher).fetch("/api/events")).rejects.toBe(error);
  expect(fetcher).toHaveBeenCalledTimes(1);
});

it.each(["navigation", "identity"])("does not retry a transport rejection after %s cancellation", async (kind) => {
  const held = deferred<Response>();
  const fetcher = vi.fn().mockReturnValue(held.promise);
  const reads = createIdentityReads(fetcher);
  const controller = new AbortController();
  const result = reads.fetch("/api/events", { signal: controller.signal }).catch((error: unknown) => error);
  if (kind === "identity") reads.reset(); else controller.abort();
  held.reject(new TypeError("Failed to fetch"));
  expect(await result).toMatchObject({ name: kind === "identity" ? "Error" : "TypeError" });
  expect(fetcher).toHaveBeenCalledTimes(1);
});
