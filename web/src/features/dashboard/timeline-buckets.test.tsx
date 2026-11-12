// Source: docklet-next/issues/435
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proofs for 15-minute timeline slots and the bounded events window notice (L3-c, L3-j).

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import type { Host, OpsEvent } from "@/domain/model";
import { activityStore } from "@/features/activity/activity-store";
import { renderWithApp } from "@/test/render";

import { timelineBuckets } from "./widgets/timeline-buckets";
import { TimelineWidget } from "./widgets/TimelineWidget";

const now = Date.parse("2026-11-11T12:00:00Z");
const event = (id: string, minutesAgo: number, hostId = "a"): OpsEvent => ({ id, at: new Date(now - minutesAgo * 60_000).toISOString(), kind: "restart", level: "info", title: `Event ${id}`, hostId });
const hosts = [{ id: "a", name: "alpha", color: "#336699", status: "online" }, { id: "b", name: "beta", color: "#993366", status: "online" }] as Host[];
afterEach(() => { setAppLocale("de"); activityStore.set((s) => ({ ...s, open: false, tab: "actions" })); });

it("L3-j groups a busy lane into at most 96 slots whose counts sum to the window's events, without future or older ones", () => {
  const busy = Array.from({ length: 1_000 }, (_, i) => event(`a${i}`, (i * 1.43) % 1_440));
  const mixed = [...busy, event("b1", 3, "b"), event("b2", 7, "b"), event("future", -5), event("old", 24 * 60 + 1), { ...event("hub", 1), hostId: undefined }];
  const input = Object.freeze(mixed.map((e) => Object.freeze(e)));
  const lanes = timelineBuckets(input, ["a", "b"], now);
  const a = lanes.get("a")!;
  expect(a.length <= 96).toBe(true);
  expect(a.reduce((n, b) => n + b.events.length, 0)).toBe(1_000);
  expect(a.some((b) => b.events.some((e) => e.id === "future" || e.id === "old"))).toBe(false);
  expect(lanes.get("b")!.map((b) => b.events.map((e) => e.id))).toEqual([["b1", "b2"]]);
  // Newest slot last (right), newest entry first within a slot.
  expect(a.at(-1)!.left > a[0]!.left).toBe(true);
  expect(a.every((b) => b.events.every((e, i) => i === 0 || Date.parse(b.events[i - 1]!.at) >= Date.parse(e.at)))).toBe(true);
});

it("L3-j renders one focusable marker per non-empty slot with its count", () => {
  setAppLocale("en-US");
  const events = [event("1", 1), event("2", 2), event("3", 20), event("4", 40, "b")];
  renderWithApp(<TimelineWidget events={events} hosts={hosts} now={now} total={4} />);
  const labels = screen.getAllByRole("button").map((b) => b.getAttribute("aria-label") ?? "");
  expect(labels.length).toBe(3);
  expect(labels.some((label) => label.includes("2 events · Event 1"))).toBe(true);
});

it("L3-c exactly 500 recent events show no notice; 501 show the limit and open the full history", async () => {
  setAppLocale("en-US");
  const events = [event("1", 1)];
  const view = renderWithApp(<TimelineWidget events={events} hosts={hosts} now={now} total={500} />);
  expect(screen.queryByText(/500\+ events/) === null).toBe(true);
  view.unmount();
  renderWithApp(<TimelineWidget events={events} hosts={hosts} now={now} total={501} />);
  expect(screen.getByText("500+ events in the last 24 hours") !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Show all events" }));
  expect([activityStore.get().open, activityStore.get().tab]).toEqual([true, "events"]);
});

it("L3-c the German notice names the limit", () => {
  setAppLocale("de");
  renderWithApp(<TimelineWidget events={[event("1", 1)]} hosts={hosts} now={now} total={900} />);
  expect(screen.getByText("Mehr als 500 Ereignisse in den letzten 24 Stunden") !== null).toBe(true);
  expect(screen.getByRole("button", { name: "Alle Ereignisse anzeigen" }) !== null).toBe(true);
});
