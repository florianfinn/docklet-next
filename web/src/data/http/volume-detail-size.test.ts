// Source: docklet-next/issues/394
// Revision: 0765af96aad9fb8e2ee6889e6d6e927cd64a3fa0
// Retrieved: 2026-11-14
// License: Apache-2.0
// Deviations: Original bounded size wait regression for the volume detail (M26 early smoke on a large host).

import { afterEach, expect, it, vi } from "vitest";
import { resourceFixture } from "./resources.test-support";
import { VOLUME_DETAIL_SIZE_WAIT_MS } from "./volumes";

afterEach(() => { vi.useRealTimers(); });

// A host-wide size scan that does not answer must not hold the detail: it ends after its own wait and the size
// stays unknown, while a prompt scan still supplies the size.
it("bounds the optional size scan of a volume detail", async () => {
  const f = resourceFixture();
  const original = f.fetcher.getMockImplementation()!;
  let sizeAborted = false;
  f.fetcher.mockImplementation(async (input: string | URL | Request, init?: RequestInit) => {
    if (String(input).endsWith("/volumes/_collection/sizes")) {
      return new Promise<Response>((_, reject) => init?.signal?.addEventListener("abort", () => { sizeAborted = true; reject(new DOMException("aborted", "AbortError")); }));
    }
    return original(input);
  });
  vi.useFakeTimers({ toFake: ["setTimeout", "clearTimeout"] });
  let settled = false;
  const detail = f.adapter.getVolume("first~data").finally(() => { settled = true; });
  await vi.advanceTimersByTimeAsync(VOLUME_DETAIL_SIZE_WAIT_MS - 1);
  expect(settled).toBe(false);
  await vi.advanceTimersByTimeAsync(1);
  expect(settled).toBe(true);
  expect(sizeAborted).toBe(true);
  expect(await detail).toMatchObject({ id: "first~data", backups: [] });
  expect((await detail).sizeBytes).not.toBe(512);
  vi.useRealTimers();
  f.fetcher.mockImplementation(original);
  expect(await f.adapter.getVolume("first~data")).toMatchObject({ sizeBytes: 512, sizeUnavailable: false });
});
