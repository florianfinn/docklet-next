// Source: docklet-next/docs/design/images-updates-identity.md
// Revision: fcf8c8d3a079154c299912a7e22e0e903e5e4e38
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Review correction with real producer and behavioral guard proofs.

import { describe, expect, it } from "vitest";
import fixture from "./testdata/unset-reads.json";
import { baseline, points, policy } from "./update-decoders";
import { observation } from "./update-validation";

describe("shared registered-handler unset reads", () => {
  it("decodes the absent baseline, policy, points and unchecked inventory", () => {
    expect(baseline(fixture.baseline)).toMatchObject({ available: false, code: "update.baseline-unavailable" });
    expect(policy(fixture.policy, "0", "example-stack")).toMatchObject({ revision: 0, mode: "check", candidates: [] });
    expect(points(fixture.points, "example-project")).toEqual([]);
    const unchecked = observation(fixture.unchecked["registry.example.com/app:stable"]);
    expect(unchecked).toMatchObject({ state: "unchecked", hasUpdate: false, authState: "unknown" });
    expect(fixture.unchecked).not.toHaveProperty("registry.example.com/absent:stable");
  });

});
