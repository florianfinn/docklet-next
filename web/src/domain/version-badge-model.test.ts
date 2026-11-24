// Source: docklet-next/issues/616
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original version badge model table (M40 break-map rows 1 and 4) and host update anchors.

import { describe, expect, it } from "vitest";
import { hostUpdateHref, isHostUpdateHash, versionBadgeModel, type SelfUpdateCheck } from "./self-update";

const check = (state: SelfUpdateCheck["eligibility"]["state"], version = "1.5.0", candidate?: string, code?: SelfUpdateCheck["eligibility"]["code"]): SelfUpdateCheck =>
  ({ version, eligibility: { state, manual: [], ...(candidate ? { candidate: { version: candidate } } : {}), ...(code ? { code } : {}) } });

describe("versionBadgeModel", () => {
  it.each([
    ["available with candidate", check("available", "1.5.0", "1.6.0"), { installed: "1.5.0", tone: "outdated", target: "1.6.0" }],
    ["current", check("current", "1.6.0"), { installed: "1.6.0", tone: "current" }],
    ["not-eligible with candidate (requires hub update)", check("not-eligible", "1.5.0", "1.7.0", "update.requires-hub-update"), { installed: "1.5.0", tone: "neutral", reason: "not-eligible" }],
    ["unavailable", check("unavailable"), { installed: "1.5.0", tone: "neutral", reason: "check-failed" }],
    ["checking", check("checking"), { installed: "1.5.0", tone: "neutral", reason: "checking" }],
    ["available without candidate", check("available"), { installed: "1.5.0", tone: "neutral", reason: "not-eligible" }],
    ["v-prefixed hub version", check("current", "v1.6.0"), { installed: "1.6.0", tone: "current" }],
  ])("row 1: %s", (_, value, expected) => {
    expect(versionBadgeModel({ reported: "1.5.0", check: value, status: "success", allowed: true })).toEqual(expected);
  });
  it.each([
    ["pending", "pending", { installed: "1.5.0", tone: "neutral", reason: "checking" }],
    ["error keeps earlier data neutral", "error", { installed: "1.5.0", tone: "neutral", reason: "check-failed" }],
    ["not requested", "off", { installed: "1.5.0", tone: "neutral", reason: "not-checked" }],
  ] as const)("row 1: %s never shows a positive tone", (_, status, expected) => {
    expect(versionBadgeModel({ reported: "1.5.0", check: check("available", "1.5.0", "1.6.0"), status, allowed: true })).toEqual(expected);
  });
  it("row 2: without the capability a cached result stays hidden and gives no reason", () => {
    expect(versionBadgeModel({ reported: "1.5.0", check: check("available", "1.5.0", "1.6.0"), status: "success", allowed: false })).toEqual({ installed: "1.5.0", tone: "neutral" });
  });
  it("row 4: a successful check decides the installed version in both mismatch directions", () => {
    expect(versionBadgeModel({ reported: "1.5.0", check: check("current", "1.6.0"), status: "success", allowed: true })).toEqual({ installed: "1.6.0", tone: "current" });
    expect(versionBadgeModel({ reported: "1.6.0", check: check("available", "1.5.0", "1.6.0"), status: "success", allowed: true })).toEqual({ installed: "1.5.0", tone: "outdated", target: "1.6.0" });
  });
  it("missing and development versions stay as reported", () => {
    expect(versionBadgeModel({ reported: "", status: "off", allowed: false })).toEqual({ installed: undefined, tone: "neutral" });
    expect(versionBadgeModel({ reported: "dev", status: "off", allowed: false }).installed).toBe("dev");
  });
  it("row 10: the model does not edit the shared check", () => {
    const value = check("available", "v1.5.0", "1.6.0"), copy = structuredClone(value);
    versionBadgeModel({ reported: "1.5.0", check: value, status: "success", allowed: true });
    expect(value).toEqual(copy);
  });
});

describe("host update anchors", () => {
  it("keeps the hub anchor and encodes agent ids", () => {
    expect(hostUpdateHref("0")).toBe("/settings/updates#hub-update");
    expect(hostUpdateHref("a b")).toBe("/settings/updates#agent-update-a%20b");
    expect(isHostUpdateHash("a b", "#agent-update-a%20b")).toBe(true);
    expect(isHostUpdateHash("a", "#agent-update-ab")).toBe(false);
    expect(isHostUpdateHash("0", "#hub-update")).toBe(true);
    expect(isHostUpdateHash("x", "#agent-update-%E0")).toBe(false);
  });
});
