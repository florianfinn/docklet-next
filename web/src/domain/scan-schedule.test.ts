import { describe, expect, it } from "vitest";

import { nextScan, normalizeScanSchedule, validateScanSchedule } from "./scan-schedule";
import { zonedParts } from "./schedule";

const BERLIN = "Europe/Berlin";

describe("scan schedule", () => {
  it("reads the former recurrence names as typed schedules", () => {
    expect(normalizeScanSchedule("manual")).toEqual({ mode: "manual" });
    expect(normalizeScanSchedule("daily")).toEqual({ mode: "daily", time: "03:00" });
    expect(normalizeScanSchedule("weekly")).toEqual({ mode: "weekly", day: "sun", time: "03:00" });
    expect(normalizeScanSchedule({ mode: "weekly", day: "wed", time: "05:15" })).toEqual({ mode: "weekly", day: "wed", time: "05:15" });
    expect(normalizeScanSchedule({ mode: "once", date: "2026-10-20", time: "03:00" })).toEqual({ mode: "daily", time: "03:00" });
    expect(normalizeScanSchedule(42)).toEqual({ mode: "daily", time: "03:00" });
  });

  it("needs a valid time, and a valid weekday when weekly; manual needs nothing", () => {
    const now = new Date("2026-10-12T10:00:00Z");
    expect(validateScanSchedule({ mode: "manual" }, now, BERLIN)).toBe(null);
    expect(validateScanSchedule({ mode: "daily", time: "03:00" }, now, BERLIN)).toBe(null);
    expect(validateScanSchedule({ mode: "daily", time: "" }, now, BERLIN)).toBe("time");
    expect(validateScanSchedule({ mode: "daily", time: "24:00" }, now, BERLIN)).toBe("time");
    expect(validateScanSchedule({ mode: "weekly", day: "xyz" as "mon", time: "03:00" }, now, BERLIN)).toBe("day");
    expect(validateScanSchedule({ mode: "weekly", day: "sun", time: "03:00", from: "2026-02-30" }, now, BERLIN)).toBe("date");
  });

  it("plans no scan while off, manual or invalid", () => {
    const now = new Date("2026-10-12T10:00:00Z");
    expect(nextScan({ mode: "daily", time: "03:00" }, false, now, BERLIN)).toBe(undefined);
    expect(nextScan({ mode: "manual" }, true, now, BERLIN)).toBe(undefined);
    expect(nextScan({ mode: "daily", time: "3 Uhr" }, true, now, BERLIN)).toBe(undefined);
  });

  it("computes daily and weekly runs as wall-clock times of the hub zone, not UTC", () => {
    // Monday 12 October 2026, 12:00 in Berlin (CEST, UTC+2).
    const now = new Date("2026-10-12T10:00:00Z");
    expect(nextScan({ mode: "daily", time: "03:00" }, true, now, BERLIN)?.toISOString()).toBe("2026-10-13T01:00:00.000Z");
    expect(nextScan({ mode: "daily", time: "13:30" }, true, now, BERLIN)?.toISOString()).toBe("2026-10-12T11:30:00.000Z");
    expect(nextScan({ mode: "weekly", day: "sun", time: "04:00" }, true, now, BERLIN)?.toISOString()).toBe("2026-10-18T02:00:00.000Z");
    expect(nextScan({ mode: "weekly", day: "sun", time: "04:00" }, true, now, "America/New_York")?.toISOString()).toBe("2026-10-18T08:00:00.000Z");
    // Today's weekday after the time has passed moves to next week.
    expect(nextScan({ mode: "weekly", day: "mon", time: "09:00" }, true, now, BERLIN)?.toISOString()).toBe("2026-10-19T07:00:00.000Z");
  });

  it("handles daylight saving changes in the hub zone", () => {
    // Autumn: 25 October 2026, the hour 02:00–03:00 occurs twice; the run keeps its wall-clock time.
    const autumn = nextScan({ mode: "daily", time: "02:30" }, true, new Date("2026-10-24T12:00:00Z"), BERLIN)!;
    expect(zonedParts(autumn, BERLIN).date).toBe("2026-10-25");
    expect(zonedParts(autumn, BERLIN).time).toBe("02:30");
    // After the change the same time is one hour later in UTC (CET, UTC+1).
    const after = nextScan({ mode: "daily", time: "03:00" }, true, new Date("2026-10-26T12:00:00Z"), BERLIN)!;
    expect(after.toISOString()).toBe("2026-10-27T02:00:00.000Z");
    // Spring: 29 March 2026 has no 02:30; the run moves by the skipped hour to 03:30 CEST.
    const spring = nextScan({ mode: "daily", time: "02:30" }, true, new Date("2026-03-28T12:00:00Z"), BERLIN)!;
    expect(spring.toISOString()).toBe("2026-03-29T01:30:00.000Z");
    expect(zonedParts(spring, BERLIN).time).toBe("03:30");
  });
});
