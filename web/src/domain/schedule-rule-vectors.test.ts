// Source: docklet-next/issues/346
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original exact-instant parity against the shared Go/web vectors.

import { readFileSync } from "node:fs";
import { it, expect } from "vitest";
import { nextScheduleExpression } from "./schedule-expression";
import { nextOccurrence } from "./schedule";
const vectors = JSON.parse(readFileSync("../types/settings/testdata/schedule-rule-vectors.json", "utf8")) as { name: string; expression: string; zone: string; after: string; next: string[] }[];
it.each(vectors)("SR11 shared vector $name", (v) => {
  let after = new Date(v.after);
  for (const expected of v.next) {
    const actual = nextScheduleExpression(v.expression, after, v.zone);
    expect(actual?.toISOString() ?? "").toBe(expected ? new Date(expected).toISOString() : "");
    const fixedDaily = /^0 (\d+) (\d+) \* \* \*$/.exec(v.expression);
    if (fixedDaily) expect(nextOccurrence({ mode: "daily", time: `${fixedDaily[2]!.padStart(2, "0")}:${fixedDaily[1]!.padStart(2, "0")}` }, after, v.zone)?.toISOString()).toBe(actual?.toISOString());
    if (actual) after = actual;
  }
});
