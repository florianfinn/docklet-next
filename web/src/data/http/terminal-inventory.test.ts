// Source: docklet-next/issues/341
// Revision: 8c86f381fb4c582fb90b56a1d158585600e2d41f
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Every terminal stack runtime run refreshes the stack inventory.

import { expect, it, vi } from "vitest";
import { activityFixture } from "./activity-reader.test-support";

it.each(["project_down", "project_restart", "project_deploy", "project_destroy"])("a terminal %s run refreshes the stack inventory", async (type) => {
  const f = activityFixture(); await f.adapter.listActions();
  const changes = vi.fn(); const stop = f.adapter.subscribeChanges(changes);
  try {
    const activity = { id: "run-stop", environmentId: "good", type, status: "success", startedAt: "2026-11-05T00:00:00Z", endedAt: "2026-11-05T00:00:01Z", resourceType: "project", resourceId: "example", resourceName: "Example", messageCode: "activity.success" };
    f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-11-05T00:00:01Z", activity: { type: "updated", environmentId: "good", activity } }), lastEventId: "" } as MessageEvent);
    expect(changes.mock.calls.some(([topics]) => topics.includes("stacks"))).toBe(true);
  } finally { stop(); }
});
