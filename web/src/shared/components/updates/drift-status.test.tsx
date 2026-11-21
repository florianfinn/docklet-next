// Source: docklet-next/docs/design/updates-ui.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #555 tests for revisionless drift statuses of older agents.

import { screen, waitFor } from "@testing-library/react";
import { expect, it } from "vitest";
import { renderWithApp } from "@/test/render";
import { catalogs } from "@/app/i18n/catalog";
import { StackUpdateDialog } from "./stack-update-dialog";
import { baseline } from "@/data/http/update-decoders";
import { updateFixture, point } from "@/data/http/stack-updates.test-support";
import { readFileSync } from "node:fs";

const t = catalogs.de.common.stackUpdates;
// The exact answer of agents before M35 after an external env file change.
// The shared fixture holds that body and the current producer shape; the hub forwards both unchanged (bootstrap test).
const shared = JSON.parse(readFileSync("../types/updater/testdata/drift-baseline.json", "utf8")) as Record<"legacy" | "fixed", { data: Record<string, unknown> }>;
const legacy = { targetIdentity: "f155c7cd0819990dc51c6b1ee8c8ee0819d8ee4076560b91f873460fae7cff16", autoExcluded: false, savedRevision: "", observedRevision: "", available: false, code: "update.config-drift" };

async function legacyFixture() {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  Object.assign(f.state.baseline, { ...legacy, targetIdentity: f.state.baseline.targetIdentity });
  return f;
}

it("decodes a revisionless unavailable status only with a known reason", () => {
  expect(baseline(legacy).code).toBe("update.config-drift");
  expect(baseline({ ...legacy, code: "update.baseline-unavailable" }).savedRevision).toBe("");
  for (const bad of [{ ...legacy, available: true }, { ...legacy, code: undefined }, { ...legacy, code: "update.check-unavailable" }, { ...legacy, savedRevision: "bad" }, { ...legacy, targetIdentity: "" }])
    expect(() => baseline(bad)).toThrow();
});

it("decodes both shared drift bodies and tolerates omitted revisions only when unavailable with a known reason", () => {
  expect(baseline(shared.legacy.data)).toMatchObject({ savedRevision: "", observedRevision: "", available: false, code: "update.config-drift" });
  expect(baseline(shared.fixed.data)).toMatchObject({ savedRevision: shared.fixed.data.savedRevision, observedRevision: shared.fixed.data.observedRevision, available: false });
  const { savedRevision: _s, observedRevision: _o, ...bare } = legacy;
  for (const omitted of [{ ...bare, observedRevision: "" }, { ...bare, savedRevision: "" }, bare]) { expect(() => baseline(omitted)).not.toThrow(); expect(baseline(omitted)).toMatchObject({ savedRevision: "", observedRevision: "" }); }
  for (const bad of [{ ...bare, available: true }, { ...bare, code: "update.check-unavailable" }, { ...bare, savedRevision: 42 }, { ...bare, savedRevision: null }, { ...bare, targetIdentity: undefined }])
    expect(() => baseline(bad)).toThrow();
});

it("an older agent's drift status shows the reason and Deploy now, never an update submit", async () => {
  const f = await legacyFixture();
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  await screen.findByText(catalogs.de.images.errors["update_config-drift"]);
  expect(screen.queryByRole("alert") === null).toBe(true);
  expect(screen.queryByRole("button", { name: t.update }) === null).toBe(true);
  await waitFor(() => expect(screen.getByRole("button", { name: t.deployNow }).hasAttribute("disabled")).toBe(false));
  expect(f.writes()).toHaveLength(0);
});

it("restore against a revisionless status offers no consent and sends nothing", async () => {
  const f = await legacyFixture();
  renderWithApp(<StackUpdateDialog stackId="direct~project" point={point} onClose={() => {}} />, f.adapter);
  await screen.findByText(catalogs.de.images.errors["update_config-drift"]);
  expect(screen.queryByRole("checkbox") === null).toBe(true);
  expect(screen.queryByText(t.restoreNeedsRevision) !== null).toBe(true);
  expect(screen.getByRole("button", { name: t.restore }).hasAttribute("disabled")).toBe(true);
  expect(f.writes()).toHaveLength(0);
});
