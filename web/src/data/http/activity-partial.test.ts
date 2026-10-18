// Source: docklet-next/issues/102
// Revision: 28f968e594b730c4960f57296abf7c44158c28c5
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original per-page partial activity metadata and retention regressions.

import { expect, it, vi } from "vitest";
import type { ActionReadHealth } from "../adapter";
import type { CodedActionRun } from "../message-code";
import { activity, activityFixture, deferred, detail, json } from "./activity-reader.test-support";

function partial(response: Response) {
  response.headers.set("X-Docklet-Host-Reason", "agent.unavailable");
  return response;
}
for (const edge of [false, true]) for (const reverse of [false, true]) {
  it(`keeps first partial hub summaries and retained agent rows (Edge fixture=${edge}, reverse=${reverse})`, async () => {
    const f = activityFixture(edge, reverse);
    f.state.lists.bad = [activity("bad", "hub-summary")];
    f.state.details["hub-summary"] = detail("bad", "hub-summary");
    f.state.hold = async (path, response) => path.endsWith("/bad/activities") ? partial(response) : response;
    const health = vi.fn<(h: ActionReadHealth) => void>();
    const runs = vi.fn<(r: CodedActionRun[]) => void>();
    const stops = [f.adapter.subscribeActionReadHealth!(health), f.adapter.subscribeActions(runs)];
    try {
      expect((await f.adapter.listActions()).map((r) => r.id).sort()).toEqual(["hub-summary", "run-good"]);
      expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
      f.state.hold = undefined;
      f.state.lists.bad!.push(activity("bad"));
      await f.adapter.listActions();
      expect(health.mock.lastCall?.[0].failures).toEqual([]);
      const retained = runs.mock.lastCall![0].find((r) => r.id === "run-bad")!;
      f.state.lists.bad = [activity("bad", "hub-summary")];
      f.state.details["hub-summary"]!.activity.status = "queued";
      f.state.hold = async (path, response) => path.endsWith("/bad/activities") ? partial(response) : response;
      const result = await f.adapter.listActions();
      expect(result.find((r) => r.id === "run-bad")).toEqual(retained);
      expect(result.find((r) => r.id === "hub-summary")?.status).toBe("queued");
      expect([...runs.mock.lastCall![0]].sort((a, b) => a.id.localeCompare(b.id))).toEqual([...result].sort((a, b) => a.id.localeCompare(b.id)));
      expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
      f.state.hold = undefined;
      expect((await f.adapter.listActions()).map((r) => r.id).sort()).toEqual(["hub-summary", "run-good"]);
      expect(health.mock.lastCall?.[0].failures).toEqual([]);
    } finally { stops.forEach((stop) => stop()); }
  });
}
it.each([0, 1])("any marked page makes the whole source incomplete (marked page %s)", async (marked) => {
  const f = activityFixture();
  await f.adapter.listActions();
  const health = vi.fn(); const off = f.adapter.subscribeActionReadHealth!(health);
  const starts: string[] = [];
  f.state.hold = async (path, response, url) => {
    if (!path.endsWith("/bad/activities")) return response;
    const start = Number(url.searchParams.get("start")); starts.push(String(start));
    const row = activity("bad", `summary-${start}`);
    f.state.details[row.id] = detail("bad", row.id);
    const page = json([row], 200, 2);
    return start === marked ? partial(page) : page;
  };
  try {
    expect((await f.adapter.listActions()).map((r) => r.id).sort()).toEqual(["run-bad", "run-good", "summary-0", "summary-1"]);
    expect(starts).toEqual(["0", "1"]);
    expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
    f.state.hold = undefined; f.state.lists.bad = [];
    expect((await f.adapter.listActions()).map((r) => r.id)).toEqual(["run-good"]);
    expect(health.mock.lastCall?.[0].failures).toEqual([]);
  } finally { off(); }
});
it.each([false, true])("marked empty pages never certify complete success (retained=%s)", async (retained) => {
  const f = activityFixture();
  if (retained) await f.adapter.listActions();
  const health = vi.fn(); const off = f.adapter.subscribeActionReadHealth!(health);
  f.state.lists.bad = [];
  f.state.hold = async (path, response) => path.endsWith("/bad/activities") ? partial(response) : response;
  try {
    expect((await f.adapter.listActions()).map((r) => r.id).sort()).toEqual(retained ? ["run-bad", "run-good"] : ["run-good"]);
    expect(health.mock.lastCall?.[0].failures).toHaveLength(1);
    f.state.hold = undefined;
    expect((await f.adapter.listActions()).map((r) => r.id)).toEqual(["run-good"]);
    expect(health.mock.lastCall?.[0].failures).toEqual([]);
  } finally { off(); }
});
it("concurrent hosts keep response metadata scoped while a healthy response completes last", async () => {
  const f = activityFixture();
  const held = deferred<Response>(); const reached = deferred<Response>(); const partialRead = deferred<void>();
  f.state.hold = async (path, response) => {
    if (path.endsWith("/good/activities")) { reached.resolve(response); return held.promise; }
    if (path.endsWith("/bad/activities/run-bad")) partialRead.resolve();
    return path.endsWith("/bad/activities") ? partial(response) : response;
  };
  const health = vi.fn(); const off = f.adapter.subscribeActionReadHealth!(health);
  const pending = f.adapter.listActions();
  const healthy = await reached.promise;
  await partialRead.promise;
  // A later healthy host response must not overwrite the other host's marked state.
  held.resolve(healthy);
  try {
    expect(await pending).toHaveLength(2);
    expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: "unavailable" }]);
  } finally { off(); }
});
it.each([401, 403, 502])("a partial header cannot downgrade HTTP %s into successful rows", async (status) => {
  const f = activityFixture(); const health = vi.fn(); const off = f.adapter.subscribeActionReadHealth!(health);
  f.state.hold = async (path, response) => path.endsWith("/bad/activities") ? partial(json([], status)) : response;
  try {
    if (status === 401) await expect(f.adapter.listActions()).rejects.toMatchObject({ outcome: { status: 401 } });
    else {
      expect((await f.adapter.listActions()).map((r) => r.id)).toEqual(["run-good"]);
      expect(health.mock.lastCall?.[0].failures).toEqual([{ hostId: "bad", reason: status === 403 ? "permission" : "unavailable" }]);
    }
  } finally { off(); }
});
