// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { expect, it } from "vitest";
import { classifyLevel } from "./level";
import { mapLog } from "@/data/http/log-mapping";

it.each([
  ['{"level":"error","event":"x"}', "error"], ['{"lvl":"trace"}', "debug"], ['{"severity":"fatal"}', "error"], ['{"log.level":"warning"}', "warn"],
  ['level=info msg="error"', "info"], ['lvl="warn" msg="error"', "warn"],
  ['2026-11-27T05:00:00Z INFO [vpn] stopping without giving any error', "info"],
  ['{"level":"info","logger":"gunicorn.error","event":"x"}', "info"],
  ['2026/11/27 05:00:00 [W] [proxy] read error: closed', "warn"],
  ['[Error] closed', "error"], ['fail: closed', "error"], ['warn: closed', "warn"], ['info: error ignored', "info"],
  ['stderr', "info"], ['an exception happened', "error"], ['warning today', "warn"],
])("classifies producer wire info: %s", (message, expected) => {
  expect(classifyLevel(message)).toBe(expected);
  for (const wire of ["info", undefined, "unknown"]) expect(mapLog({ seq: 1, message, level: wire }, "scope", 1, "web", () => "2026-01-01T00:00:00Z").level).toBe(expected);
});
it.each(["warn", "error", "debug"])("keeps authoritative wire %s", (level) => {
  expect(mapLog({ seq: 1, message: "source closed", level }, "scope", 1, "web", () => "2026-01-01T00:00:00Z").level).toBe(level);
});
