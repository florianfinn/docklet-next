// Source: docklet-next/issues/235
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original product version precedence tests mirroring types/version.

// @vitest-environment node
import { describe, expect, it } from "vitest";
import { compareProductVersion } from "./product-version";

describe("compareProductVersion", () => {
  it("follows the SemVer 2 precedence chain", () => {
    const chain = ["1.0.0-alpha", "1.0.0-alpha.1", "1.0.0-alpha.beta", "1.0.0-beta", "1.0.0-beta.2", "1.0.0-beta.11", "1.0.0-rc.1", "1.0.0-rc.2", "1.0.0-rc.10", "1.0.0", "1.0.1", "1.1.0", "2.0.0"];
    chain.forEach((a, i) => chain.forEach((b, j) => expect(compareProductVersion(a, b)).toBe(Math.sign(i - j))));
    expect(compareProductVersion("v1.0.0+build.1", "1.0.0")).toBe(0);
  });

  it("rejects non-SemVer values", () => {
    for (const raw of ["", "dev", "1.0", "01.0.0", "1.0.0-", "1.0.0-rc..1", "1.0.0-rc.01", "1.0.0+", "latest"]) expect(compareProductVersion(raw, "1.0.0")).toBeUndefined();
  });
});
