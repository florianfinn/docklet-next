// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original frozen self-update model regression tests.

import { describe, expect, it } from "vitest";
import { canSelfUpdate, selfUpdateState, selfUpdateRef, validSelfUpdateCheck } from "./self-update";
import type { SelfUpdateCheck } from "./self-update";

describe("self-update state", () => {
  it.each(["available", "current", "checking", "not-eligible", "unavailable"] as const)("preserves %s and enables only an available candidate", state => {
    const check: SelfUpdateCheck = { version: "1.0.0", eligibility: { state, candidate: { version: "1.0.1" }, manual: ["docker compose pull"] } };
    expect(validSelfUpdateCheck(check)).toBe(true);
    const mapped = selfUpdateState(check);
    expect(mapped.state).toBe(state);
    expect(canSelfUpdate(mapped)).toBe(state === "available");
    expect(mapped.manual).toEqual(["docker compose pull"]);
  });
  it("accepts nullable producer commands and refuses an incomplete available candidate", () => {
    expect(selfUpdateState().state).toBe("checking");
    const row = { version: "1.0.0", eligibility: { state: "current", manual: null } } as const;
    expect(validSelfUpdateCheck(row)).toBe(true); expect(selfUpdateState(row).manual).toEqual([]);
    expect(validSelfUpdateCheck({ ...row, eligibility: { ...row.eligibility, state: "available" } })).toBe(false);
    expect(validSelfUpdateCheck({ ...row, eligibility: { ...row.eligibility, state: "new-state" } } as unknown as SelfUpdateCheck)).toBe(false);
    expect(validSelfUpdateCheck(null as unknown as SelfUpdateCheck)).toBe(false);
  });
  it("uses the exact hub and agent capability resources", () => {
    expect(selfUpdateRef("0")).toEqual({ kind: "hub", id: "0" });
    expect(selfUpdateRef("example")).toEqual({ kind: "host", id: "example" });
  });
});
