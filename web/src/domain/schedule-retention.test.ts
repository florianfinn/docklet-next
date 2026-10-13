import { describe, expect, it } from "vitest";

import { applyRetention, normalizeRetention, validateRetention } from "./retention";
import { nextOccurrence, parseDate, parseLegacyWindow, validateSchedule, weekdayOf, withRecurrence, zonedParts, zonedTimeToInstant } from "./schedule";

const TZ = "Europe/Berlin";
const at = (d: Date | undefined, tz = TZ) => (d ? `${zonedParts(d, tz).date} ${zonedParts(d, tz).time}` : undefined);

describe("schedule model", () => {
  it("computes the next run in the hub time zone, not in UTC or the browser zone", () => {
    const now = new Date("2026-10-11T20:30:00Z"); // Sunday 22:30 in Berlin, 06:30 Monday in Tokyo
    expect(at(nextOccurrence({ mode: "daily", time: "23:00" }, now, TZ))).toBe("2026-10-11 23:00");
    expect(at(nextOccurrence({ mode: "daily", time: "23:00" }, now, "Asia/Tokyo"), "Asia/Tokyo")).toBe("2026-10-12 23:00");
    expect(at(nextOccurrence({ mode: "weekly", day: "wed", time: "03:30" }, now, TZ))).toBe("2026-10-14 03:30");
    // Same weekday, time already passed: one week later.
    expect(at(nextOccurrence({ mode: "weekly", day: "sun", time: "04:00" }, now, TZ))).toBe("2026-10-18 04:00");
  });

  it("handles daylight saving changes and gaps", () => {
    // Berlin switches to CET on 2026-10-25: 03:30 local is 02:30 UTC afterwards.
    expect(zonedTimeToInstant("2026-10-25", "03:30", TZ).toISOString()).toBe("2026-10-25T02:30:00.000Z");
    expect(zonedTimeToInstant("2026-10-24", "03:30", TZ).toISOString()).toBe("2026-10-24T01:30:00.000Z");
    // 2027-03-28 02:30 does not exist in Berlin; it moves on by the skipped hour.
    expect(at(zonedTimeToInstant("2027-03-28", "02:30", TZ))).toBe("2027-03-28 03:30");
  });

  it("keeps weekly intent with an optional first date and validates one-time runs", () => {
    const now = new Date("2026-10-11T08:00:00Z");
    expect(at(nextOccurrence({ mode: "weekly", day: "wed", time: "03:30", from: "2026-10-21" }, now, TZ))).toBe("2026-10-21 03:30");
    expect(at(nextOccurrence({ mode: "once", date: "2026-10-13", time: "22:15" }, now, TZ))).toBe("2026-10-13 22:15");
    expect(validateSchedule({ mode: "once", date: "2026-10-10", time: "03:00" }, now, TZ)).toBe("past");
    expect(validateSchedule({ mode: "once", date: "", time: "03:00" }, now, TZ)).toBe("date");
    expect(validateSchedule({ mode: "once", date: "2026-02-30", time: "03:00" }, now, TZ)).toBe("date");
    expect(validateSchedule({ mode: "daily", time: "24:00" }, now, TZ)).toBe("time");
    expect(validateSchedule({ mode: "weekly", day: "wed", time: "03:30" }, now, TZ)).toBe(null);
    expect([parseDate("2026-02-29"), weekdayOf("2026-10-14")]).toEqual([undefined, "wed"]);
  });

  it("switches recurrence without silently turning recurring plans into one-time runs", () => {
    const now = new Date("2026-10-11T08:00:00Z"); // Sunday
    const weekly = { mode: "weekly", day: "wed", time: "03:30" } as const;
    expect(withRecurrence(weekly, "daily", now, TZ)).toEqual({ mode: "daily", time: "03:30" });
    // A new one-time run needs an explicit date; the time carries over.
    expect(withRecurrence(weekly, "once", now, TZ)).toEqual({ mode: "once", date: "", time: "03:30" });
    expect(withRecurrence({ mode: "once", date: "2026-10-16", time: "21:00" }, "weekly", now, TZ)).toEqual({ mode: "weekly", day: "fri", time: "21:00" });
    expect(withRecurrence({ mode: "manual" }, "daily", now, TZ)).toEqual({ mode: "daily", time: "03:00" });
  });

  it("reads former free-text windows", () => {
    expect(parseLegacyWindow("Mi 03:30")).toEqual({ mode: "weekly", day: "wed", time: "03:30" });
    expect(parseLegacyWindow("So 4:00")).toEqual({ mode: "weekly", day: "sun", time: "04:00" });
    expect(parseLegacyWindow("23:15")).toEqual({ mode: "daily", time: "23:15" });
    expect(parseLegacyWindow("irgendwann")).toBe(undefined);
  });
});

describe("retention model", () => {
  const GB = 1024 ** 3;
  const runs = [5, 4, 3, 2, 1, 1].map((gb, i) => ({ id: `r${i}`, sizeBytes: gb * GB }));

  it("combines count and size limits; the newest archives stay without gaps", () => {
    const ids = (r: { maxCount: number | null; maxSizeGb: number | null }) => applyRetention(runs, r).keep.map((x) => x.id);
    expect(ids({ maxCount: 3, maxSizeGb: null })).toEqual(["r0", "r1", "r2"]);
    expect(ids({ maxCount: null, maxSizeGb: 10 })).toEqual(["r0", "r1"]);
    // Both: whichever limit is reached first; an older small archive after a gap is not kept.
    expect(ids({ maxCount: 5, maxSizeGb: 12 })).toEqual(["r0", "r1", "r2"]);
    expect(ids({ maxCount: 2, maxSizeGb: 100 })).toEqual(["r0", "r1"]);
    // Unlimited keeps everything; the newest stays even when alone above the size limit.
    expect(ids({ maxCount: null, maxSizeGb: null })).toHaveLength(6);
    expect(ids({ maxCount: null, maxSizeGb: 1 })).toEqual(["r0"]);
  });

  it("validates whole numbers in range and migrates the former archive count", () => {
    expect(validateRetention({ maxCount: 0, maxSizeGb: null })).toBe("count");
    expect(validateRetention({ maxCount: 7, maxSizeGb: Number.NaN })).toBe("size");
    expect(validateRetention({ maxCount: 2.5, maxSizeGb: null })).toBe("count");
    expect(validateRetention({ maxCount: null, maxSizeGb: null })).toBe(null);
    expect(normalizeRetention(14)).toEqual({ maxCount: 14, maxSizeGb: null });
    expect(normalizeRetention({ maxCount: null, maxSizeGb: 200 })).toEqual({ maxCount: null, maxSizeGb: 200 });
    expect(normalizeRetention("x")).toEqual({ maxCount: 7, maxSizeGb: null });
  });
});
