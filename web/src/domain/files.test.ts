// Source: docklet-next/issues/330
// Revision: 20a03b67fbeb6bcc82c758e6f2ece937d58f037a
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original destination checks for the file move gesture (M16 candidate F01).

import { describe, expect, it } from "vitest";

import { moveTarget } from "./files";

describe("moveTarget", () => {
  it.each([
    ["", "/a.txt", { error: "required" }],
    ["config", "/a.txt", { error: "invalid" }],
    ["/x/../y", "/a.txt", { error: "invalid" }],
    ["/", "/a.txt", { error: "same" }],
    ["/dir/", "/dir/a.txt", { error: "same" }],
    ["/dir", "/dir", { error: "inside" }],
    ["/dir/sub", "/dir", { error: "inside" }],
    ["/dir2", "/dir", { folder: "/dir2" }],
    ["//config/./nested/", "/a.txt", { folder: "/config/nested" }],
  ])("checks %j for %s", (input, path, expected) => {
    expect(moveTarget(input, path)).toEqual(expected);
  });
});
