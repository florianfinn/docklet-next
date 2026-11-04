// Source: docklet-next/issues/297
// Revision: 693beb34e4f331cd918c2183f07b2f935a8620e7
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original delivery event and Auto Heal activity mapping tests from the hub producer shapes.

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

// Shape of a persisted hub delivery event (notification service producer), as retained in the M17 acceptance.
const sent = { ...failure({ eventType: "deploy_failed", notificationId: "n", provider: "gotify", status: "success" }), type: "notification.send", messageCode: "notification.send", severity: "success", title: "Notification sent via gotify", description: "deploy_failed", resourceName: "gotify" } as unknown as Event;

it.each([
  ["de", "Benachrichtigung über Gotify gesendet"],
  ["en-US", "Notification sent via Gotify"],
] as const)("renders a sent notification with its channel in %s", (locale, title) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    expect(renderEvent(mapEvent(sent), locale).title).toBe(title);
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

it.each([
  ["de", "Anhaltende Last", "Anhaltend hohe Ressourcenauslastung", "web wurde unerwartet beendet (Exit 137)"],
  ["en-US", "Sustained load", "Sustained resource load", "web exited unexpectedly (exit 137)"],
] as const)("renders a load alert activity and a condition event from their hub shapes in %s", (locale, title, message, condition) => {
  const errors = vi.spyOn(console, "error").mockImplementation(() => {});
  try {
    const detail = {
      activity: { id: "load", type: "load_sustained", environmentId: "host", resourceType: "host", resourceId: "host:cpu", resourceName: "host:cpu", status: "success", progress: 0, latestMessage: "load.sustained", messageCode: "load.sustained", startedAt: "2026-11-03T12:00:00Z", endedAt: "2026-11-03T12:00:00Z", createdAt: "2026-11-03T12:00:00Z" },
      messages: [], output: [], stepTrace: [], outputTruncated: false,
    } as unknown as Detail;
    const run = renderRun(mapActivityDetail(detail), locale);
    expect([run.kind, run.title, run.log.includes(message)]).toEqual(["load-alert", title, true]);
    const event = { id: "event", type: "container.condition.down", messageCode: "container.condition.down", severity: "error", title: "container.condition.down", description: "container.condition.down", resourceType: "container", resourceId: "c1", resourceName: "web", environmentId: "host", metadata: { hostId: "host", containerName: "web", exitCode: 137, restarts: 0 }, timestamp: "2026-11-03T12:00:00Z", createdAt: "2026-11-03T12:00:00Z", updatedAt: "2026-11-03T12:00:00Z" } as unknown as Event;
    expect(renderEvent(mapEvent(event), locale).title).toBe(condition);
    expect(mapEvent(event).condition).toBe(true);
    expect(errors).not.toHaveBeenCalled();
  } finally { errors.mockRestore(); }
});

it.each([
  ["de", "restart-loop", { containerName: "db", exitCode: 0, restarts: 4 }, "db befindet sich in einer Neustartschleife (4 Neustarts in 10 Min.)"],
  ["en-US", "restart-loop", { containerName: "db", exitCode: 0, restarts: 4 }, "db is in a restart loop (4 restarts in 10 min)"],
  ["en-US", "down", { containerName: "", exitCode: 0, restarts: 0 }, "web exited unexpectedly"],
  ["en-US", "running-again", { containerName: "db", exitCode: 0, restarts: 0 }, "db is running again"],
] as const)("names the container, exit code and restarts of a %s %s condition event", (locale, kind, metadata, title) => {
  const event = { id: "event", type: `container.condition.${kind}`, messageCode: `container.condition.${kind}`, severity: "warning", title: `container.condition.${kind}`, description: "", resourceType: "container", resourceId: "c1", resourceName: "web", environmentId: "host", metadata: { hostId: "host", ...metadata }, timestamp: "2026-11-03T12:00:00Z" } as unknown as Event;
  expect(renderEvent(mapEvent(event), locale).title).toBe(title);
});
