// Source: docklet-next/docs/design/stack-authoring.md
// Revision: 562b60474d0d5442a40a789bf9a95320949cd35e
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original atomic service import tests for M7 review finding M7-R1-02.

import { describe, expect, it } from "vitest";
import { parse } from "yaml";

import { ComposeEditError, mutate, ops } from "./ops";

const base = "services:\n  app:\n    image: nginx:1.29\nvolumes:\n  data: {}\n";

describe("importService", () => {
  it("adds missing declarations and keeps equal ones", () => {
    const out = parse(mutate(base, ops.importService("web", { image: "nginx:1.29", networks: ["example-net"] }, { networks: { "example-net": { external: true } }, volumes: { data: {} } })));
    expect(out.networks).toEqual({ "example-net": { external: true } });
    expect(out.volumes).toEqual({ data: {} });
    expect(out.services.web.networks).toEqual(["example-net"]);
  });

  it("refuses the whole edit when a declaration differs", () => {
    let error: unknown;
    try { mutate(base, ops.importService("web", { image: "x" }, { volumes: { data: { external: true } } })); } catch (e) { error = e; }
    expect(error instanceof ComposeEditError && error.code === "declaration-conflict" && error.detail === "data").toBe(true);
  });
});
