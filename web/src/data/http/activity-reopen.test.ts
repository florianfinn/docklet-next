// Source: docklet-next/docs/design/activity-discovery.md
// Revision: 86876feed1c0f8ad66d9a3eb54da6c4be5c3ebbc
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Abort-honoring response-body regression for bounded reopen interruption (#223).

import { expect, it, vi } from "vitest";
import { ActivitySource, activityFixture, deferred } from "./activity-reader.test-support";
import { createHttpAdapter } from "./http-adapter";

it("a reopen-only storm aborts one pre-error body and resolves transferred callers from the protected body", async () => {
  const f = activityFixture(); const source = new ActivitySource();
  let hold = false; let aborts = 0;
  const bodies: Array<{ release: () => void }> = [];
  const reached = [deferred<void>(), deferred<void>()];
  const fetcher = (async (input: RequestInfo | URL, init?: RequestInit) => {
    const response = await f.fetcher(input, init);
    if (!hold || !new URL(String(input), "https://hub.example.com").pathname.endsWith("/good/activities")) return response;
    const encoded = new TextEncoder().encode(await response.text());
    const index = bodies.length;
    return new Response(new ReadableStream({ start(controller) {
      let settled = false;
      const abort = () => { if (!settled) { settled = true; aborts++; controller.error(init?.signal?.reason); } };
      init?.signal?.addEventListener("abort", abort, { once: true });
      if (init?.signal?.aborted) abort();
      bodies.push({ release: () => { if (!settled) { settled = true; controller.enqueue(encoded); controller.close(); } } });
      reached[index]?.resolve();
    } }), { headers: response.headers });
  }) as typeof fetch;
  const adapter = createHttpAdapter({ fetch: fetcher, eventSource: () => source as unknown as EventSource });
  const runs = vi.fn(); const stop = adapter.subscribeActions(runs); vi.useFakeTimers();
  try {
    await adapter.listActions(); hold = true;
    const before = f.requests.filter((path) => path.includes("/good/activities?")).length;
    const pending = adapter.listActions(); void pending.catch(() => undefined); await reached[0]!.promise;
    f.state.lists.good![0]!.status = "queued";
    source.onerror?.({} as Event); source.open(); await reached[1]!.promise;
    for (let i = 0; i < 30; i++) { source.onerror?.({} as Event); source.open(); }
    await vi.advanceTimersByTimeAsync(0);
    expect(aborts).toBe(1); expect(bodies).toHaveLength(2);
    expect(f.requests.filter((path) => path.includes("/good/activities?")).length - before).toBe(2);
    bodies[1]!.release();
    expect((await pending).find((run) => run.id === "run-good")?.status).toBe("queued");
    expect(runs.mock.lastCall?.[0].find((run: { id: string }) => run.id === "run-good")).toMatchObject({ status: "queued" });
    expect(f.requests.filter((path) => /\/activities\/|\/runtime-runs\//.test(path))).toEqual([]);
  } finally { stop(); vi.useRealTimers(); }
});
