// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original generated-contract proxy outcome decoder.

import { readFileSync } from "node:fs";
import { render, screen, cleanup } from "@testing-library/react";
import { afterEach, describe, expect, it } from "vitest";
import {renderMessage} from "../message-code";
import { decodeProxyOperation, proxyOutcome } from "./proxy-decoder";
const fixtures = JSON.parse(readFileSync("../types/proxy/testdata/outcomes.json", "utf8")) as { transport: string; state: string; reason: string; body: unknown }[];
afterEach(cleanup);
describe("P19 proxy outcomes from real Direct and Edge forwarding", () => {
  it("preserves every recorded state and reason", () => {
    for (const row of fixtures) {
      const operation = decodeProxyOperation(row.body);
      const view = proxyOutcome(operation);
      expect(view.state, row.transport).toBe(row.state);
      expect(operation.reason ?? "", row.transport).toBe(row.reason);
      if(view.reasonKey){for(const locale of ["de","en-US"] as const)expect(renderMessage({code:view.reasonKey,params:{}},locale)).not.toBe(row.reason);}
      expect(view.tone === "success").toBe(row.state === "verified");
    }
  });
  for (const state of ["applied-unverified", "recovery-required"]) it(`renders ${state} without claiming verification`, () => {
    const body = fixtures.find((f) => f.state === state)?.body;
    const view = proxyOutcome(decodeProxyOperation(body));
    render(<output data-state={view.state} data-tone={view.tone}>{renderMessage({code:view.labelKey,params:{}},"en-US")}</output>);
    expect(screen.getByRole("status").getAttribute("data-state")).toBe(state);
    expect(screen.getByRole("status").textContent).toBe(state === "applied-unverified" ? "Applied, not verified" : "Recovery required");
    expect(screen.getByRole("status").getAttribute("data-tone")).toBe(state === "applied-unverified" ? "warning" : "danger");
  });
  it("keeps the engine diagnostic from both transports", () => {
    const rows = fixtures.filter((f) => (f.body as { data: { diagnostic?: unknown } }).data.diagnostic !== undefined);
    expect(rows.map((r) => r.transport).sort()).toEqual(["direct", "edge"]);
    for (const row of rows) expect(decodeProxyOperation(row.body).diagnostic).toEqual({ path: "routes/app.caddy", line: 3, detail: "Error: adapting config: unrecognized directive: example_directive" });
  });
  it("rejects a malformed diagnostic", () => {
    const original = fixtures[0]!.body as { success: true; data: object };
    for (const diagnostic of ["text", { line: -1 }, { line: 1.5 }, { path: 7 }, { detail: "x".repeat(4097) }, { path: "x".repeat(1025) }]) expect(() => decodeProxyOperation({ ...original, data: { ...original.data, diagnostic } })).toThrow("proxy.invalid-response");
  });
  it("rejects unknown states and reasons", () => {
    const original = fixtures[0]!.body as { success: true; data: object };
    for (const patch of [{ state: "future-state" }, { reason: "future-reason" }]) expect(() => decodeProxyOperation({ ...original, data: { ...original.data, ...patch } })).toThrow("proxy.invalid-response");
  });
});
