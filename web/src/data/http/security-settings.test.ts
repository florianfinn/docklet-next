// Source: docklet-next/issues/319
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original producer-shaped scan schedule decoding guards.

import { expect, it } from "vitest";
import { scanSchedule } from "./security-settings";

// The hub stores schedules as written once they parse (vulnerability_schedule.go), including blank keys.
it("decodes every scan schedule shape the hub stores", () => {
  expect(scanSchedule("")).toEqual({ mode: "manual" });
  expect(scanSchedule(JSON.stringify({ mode: "manual", day: "", time: "" }))).toEqual({ mode: "manual" });
  expect(scanSchedule(JSON.stringify({ mode: "manual", day: null }))).toEqual({ mode: "manual" });
  expect(scanSchedule(JSON.stringify({ mode: "daily", time: "03:00", day: null }))).toEqual({ mode: "daily", time: "03:00" });
  expect(scanSchedule(JSON.stringify({ mode: "weekly", time: "03:00", day: "mon" }))).toEqual({ mode: "weekly", time: "03:00", day: "mon" });
});

it("still rejects values that carry meaning in the wrong mode", () => {
  expect(() => scanSchedule(JSON.stringify({ mode: "manual", time: "03:00" }))).toThrow();
  expect(() => scanSchedule(JSON.stringify({ mode: "daily", time: "03:00", day: "mon" }))).toThrow();
});
