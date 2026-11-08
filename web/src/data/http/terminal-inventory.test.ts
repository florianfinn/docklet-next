// Source: docklet-next/issues/341
// Revision: 8c86f381fb4c582fb90b56a1d158585600e2d41f
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Every terminal stack runtime run refreshes the stack inventory, also when only an activity read observes the end. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271).

import { expect, it, vi } from "vitest";
import { activityFixture, at } from "./activity-reader.test-support";

it.each(["project_down", "project_restart", "project_deploy", "project_destroy"])("a terminal %s run refreshes the stack inventory", async (type) => {
  const f = activityFixture(); await f.adapter.listActions();
  const changes = vi.fn(); const off = f.adapter.subscribeChanges(() => {}); const stop = f.adapter.subscribeStackTerminals!(changes);
  try {
    const activity = { id: "run-stop", environmentId: "good", type, status: "success", createdAt: "2026-11-05T00:00:00Z", startedAt: "2026-11-05T00:00:00Z", endedAt: "2026-11-05T00:00:01Z", resourceType: "project", resourceId: "example", resourceName: "Example", messageCode: "activity.success" as const };
    f.state.details[activity.id] = { activity, messages: [], output: [], stepTrace: [], outputTruncated: false };
    f.sources[0]!.onmessage?.({ data: JSON.stringify({ channel: "activities", timestamp: "2026-11-05T00:00:01Z", activity: { type: "updated", environmentId: "good", activity } }), lastEventId: "" } as MessageEvent);
    await vi.waitFor(() => expect(changes).toHaveBeenCalled());
    expect(changes.mock.calls.some(([target]) => target.stackId === "good~example" && target.hostId === "good")).toBe(true);
  } finally { stop(); off(); }
});

it.each([["stack", true], ["container", false]] as const)("an activity read that observes a %s run end refreshes stacks: %s", async (resourceType, refreshes) => {
  const f = activityFixture();
  const running = { ...f.state.lists.good![0]!, type: "project_down", resourceType, resourceId: "example" };
  f.state.lists.good = [running];
  const runs = vi.fn(); const unsubscribe = f.adapter.subscribeActions(runs);
  const changes = vi.fn(); const stop = f.adapter.subscribeStackTerminals!(changes);
  try {
    await f.adapter.listActions();
    expect(changes.mock.calls.some(([target]) => target.stackId === "good~example" && target.hostId === "good")).toBe(false);
    f.state.lists.good = [{ ...running, status: "success", endedAt: at }];
    await f.adapter.listActions();
    expect(changes.mock.calls.some(([target]) => target.stackId === "good~example" && target.hostId === "good")).toBe(refreshes);
  } finally { stop(); unsubscribe(); }
});
