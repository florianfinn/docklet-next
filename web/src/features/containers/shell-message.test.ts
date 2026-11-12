// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shell code coverage against v11, OpenAPI and both catalogues.

import { readFileSync } from "node:fs";
import { resolve } from "node:path";
import { expect, it } from "vitest";
import { SHELL_REASONS } from "@/data/shell";
import { enContainers } from "./messages/en";
import { deContainers } from "./messages/de";
import { shellMessage } from "./shell-message";

it("maps every v11 shell code to a distinct complete EN/DE message", () => {
  const openapi = readFileSync(resolve(process.cwd(), "../api/openapi.json"), "utf8");
  const fixture = readFileSync(resolve(process.cwd(), "../types/contract/testdata/agent-hub-v11.json"), "utf8");
  const keys = SHELL_REASONS.map(shellMessage);
  expect(new Set(keys).size).toBe(SHELL_REASONS.length);
  for (const code of SHELL_REASONS) {
    expect(openapi.includes(code)).toBe(true); expect(fixture.includes(code)).toBe(true);
    expect(enContainers.terminal[shellMessage(code)].length).toBeGreaterThan(0);
    expect(deContainers.terminal[shellMessage(code)].length).toBeGreaterThan(0);
  }
  expect(shellMessage("unknown-code")).toBe("connectionEnded");
});
