// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that a read refused by hub maintenance renders as a neutral status.

import { screen } from "@testing-library/react";
import { expect, it } from "vitest";

import { AdapterOutcomeError } from "@/data/capability";
import { renderWithApp } from "@/test/render";
import { QueryError } from "./page";

it("R1B-5 a maintenance refusal renders as a status, not as a destructive error", async () => {
  renderWithApp(<QueryError error={new AdapterOutcomeError({ kind: "error", code: "backup.maintenance", status: 503 })} onRetry={() => {}} />);
  expect((await screen.findByRole("status", {}, { timeout: 2_000 })).textContent).toMatch(/maintenance|Wartung/i);
  expect(screen.queryByRole("alert")).toBeNull();
});

it("R1B-5 an ordinary read failure still renders the destructive error", async () => {
  renderWithApp(<QueryError error={new AdapterOutcomeError({ kind: "error", code: "http.error", status: 500 })} onRetry={() => {}} />);
  expect(await screen.findByRole("alert")).not.toBeNull();
});
