// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original picker conversion tests for issue #32.

import { describe, expect, it } from "vitest";

import { formatSimpleBackupSchedule, parseSimpleBackupSchedule, validBackupTime } from "./simple-schedule";

describe("simple Hub backup schedules", () => {
  it.each([
    ["0 0 3 * * *", { frequency: "daily", time: "03:00" }],
    ["0 45 23 * * 2", { frequency: "weekly", time: "23:45", weekday: "2" }],
    ["0 5 7 * * SUN", { frequency: "weekly", time: "07:05", weekday: "0" }],
    ["0 5 7 * * 7", { frequency: "weekly", time: "07:05", weekday: "0" }],
    ["00 05 07 * * 00", { frequency: "weekly", time: "07:05", weekday: "0" }],
  ] as const)("round-trips %s through the picker", (cron, expected) => {
    const parsed = parseSimpleBackupSchedule(cron);
    expect(parsed).toEqual(expected);
    expect(parseSimpleBackupSchedule(formatSimpleBackupSchedule(parsed!))).toEqual(expected);
  });

  it.each(["*/5 * * * * *", "0 0 3 1 * *", "0 0 3 * * 1,3", "0 0 24 * * *", "0 0 3 * * 8", "@daily"])("keeps %s outside the simple picker", (cron) => {
    expect(parseSimpleBackupSchedule(cron)).toBeNull();
  });

  it("rejects invalid 24-hour times", () => {
    expect(validBackupTime("23:59")).toBe(true);
    expect(validBackupTime("24:00")).toBe(false);
    expect(validBackupTime("09:60")).toBe(false);
    expect(validBackupTime("9:00")).toBe(false);
  });
});
