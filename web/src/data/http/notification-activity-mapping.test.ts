// Source: docklet-next/issues/297
// Revision: 693beb34e4f331cd918c2183f07b2f935a8620e7
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original delivery failure event and Auto Heal activity mapping tests from the hub producer shapes.

import { expect, it, vi } from "vitest";

import { renderEvent, renderRun } from "../message-code";
import { mapActivityDetail, mapEvent } from "./map-dto";

type Event = Parameters<typeof mapEvent>[0];
type Detail = Parameters<typeof mapActivityDetail>[0];

// Shape of a persisted hub delivery failure event (notification outbox producer).
const failure = (metadata: Record<string, unknown>) => ({
  id: "event", type: "notification.delivery-failed", messageCode: "notification.delivery-failed", severity: "error",
  title: "Notification failed via ntfy", description: "deploy_failed: delivery-unreachable", resourceType: "notification", resourceName: "ntfy",
  userId: "system", username: "System", environmentId: "host", metadata, timestamp: "2026-10-29T12:00:00Z", createdAt: "2026-10-29T12:00:00Z", updatedAt: "2026-10-29T12:00:00Z",
}) as unknown as Event;

it.each([
  ["de", "Benachrichtigung über ntfy fehlgeschlagen: Ziel nicht erreichbar", "Benachrichtigung über ntfy fehlgeschlagen"],
  ["en-US", "Notification via ntfy failed: Target unreachable", "Notification via ntfy failed"],
] as const)("renders a delivery failure with channel and typed reason in %s", (locale, withReason, withoutReason) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const meta = { eventType: "deploy_failed", notificationId: "n", provider: "ntfy", status: "failed" };
    expect(renderEvent(mapEvent(failure({ ...meta, reason: "delivery-unreachable" })), locale).title).toBe(withReason);
    expect(renderEvent(mapEvent(failure({ ...meta, reason: "raw sender diagnostic" })), locale).title).toBe(withoutReason);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

it.each([
  ["de", "Neustart durch Auto Heal", "Container durch Auto Heal neu gestartet"],
  ["en-US", "Auto Heal restart", "Container restarted by Auto Heal"],
] as const)("renders a settled Auto Heal restart as its own complete action in %s", (locale, title, message) => {
  const detail = {
    activity: { id: "run", type: "container_auto_heal", environmentId: "host", resourceType: "container", resourceId: "c", resourceName: "web", status: "success", progress: 0, latestMessage: "Container restarted by Auto Heal", messageCode: "autoheal.restarted", startedAt: "2026-10-29T12:00:00Z", endedAt: "2026-10-29T12:00:01Z", createdAt: "2026-10-29T12:00:00Z" },
    messages: [{ id: "m", activityId: "run", code: "autoheal.restarted", message: "Container restarted by Auto Heal", level: "info", createdAt: "2026-10-29T12:00:01Z" }],
    output: [], stepTrace: [], outputTruncated: false,
  } as unknown as Detail;
  const run = renderRun(mapActivityDetail(detail), locale);
  expect([run.kind, run.title, run.progress, run.log.includes(message)]).toEqual(["auto-heal", title, 100, true]);
});
