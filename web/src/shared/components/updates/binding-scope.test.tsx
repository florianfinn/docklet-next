// Source: docklet-next/issues/594
// Revision: 6e3c477e97192029f67bcf924da27b3b3849bbd8
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original review regression proofs: binding scope against the selection, no predicted refusal, repeatable checks.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { updateFixture } from "@/data/http/stack-updates.test-support";
import { envelope } from "@/data/http/runtime-commands.test-support";
import { AdapterOutcomeError } from "@/data/capability";
import { renderWithApp } from "@/test/render";
import { StackUpdateDialog } from "./stack-update-dialog";
import { CheckUpdatesButton } from "./check-updates-button";
import { StackBindingNotice, useStackEligibility } from "./stack-binding-notice";

const de = catalogs.de;

async function stackFixture(bound: "app" | "db") {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const demo = createDemoAdapter(), host = (await demo.listHosts())[0]!;
  host.id = "direct"; host.componentVersion = { ...host.componentVersion, agentContractVersion: 23 } as NonNullable<typeof host.componentVersion>;
  f.adapter.listHosts = async () => [host];
  const detail = await demo.getStack((await demo.listStacks())[0]!.id), template = detail.services[0]!;
  const current = { state: "current", current: { tag: "1.0.0" }, checkedAt: "2026-11-24T00:00:00Z", registry: "registry.example.com" } as unknown as typeof template.update;
  const version = { state: "version", current: { tag: "1.0.0" }, latest: { tag: "1.1.0" }, level: "minor", checkedAt: "2026-11-24T00:00:00Z", registry: "registry.example.com" } as unknown as typeof template.update;
  // app is built locally without an update; db has a pending image update.
  detail.id = "direct~project"; detail.services = [{ ...template, name: "app", update: current }, { ...template, name: "db", update: version }];
  f.adapter.getStack = async () => detail;
  f.state.hold = (path) => path.endsWith("/update-eligibility") ? Promise.resolve(envelope({ services: [
    { service: "app", eligible: bound !== "app", ...(bound === "app" ? { binding: { scope: "service", service: "app", reason: "build-service" } } : {}) },
    { service: "db", eligible: bound !== "db", ...(bound === "db" ? { binding: { scope: "service", service: "db", reason: "digest-pinned" } } : {}) }] })) : undefined;
  return f;
}

it("offers the whole-stack update when only a service without an update has a binding", async () => {
  const f = await stackFixture("app");
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  const dialog = within(await screen.findByRole("dialog"));
  await waitFor(() => expect(f.calls.some((call) => call.path.endsWith("/update-eligibility"))).toBe(true));
  await waitFor(() => expect(dialog.getByRole("button", { name: de.common.stackUpdates.update }).hasAttribute("disabled")).toBe(false));
  // The build service is still explained while the update of its peer stays offered.
  expect(dialog.queryAllByRole("status").some((node) => node.textContent?.includes("app:") === true)).toBe(true);
});

it("refuses the whole-stack update when a service with an update has a binding", async () => {
  const f = await stackFixture("db");
  renderWithApp(<StackUpdateDialog stackId="direct~project" onClose={() => {}} />, f.adapter);
  const dialog = within(await screen.findByRole("dialog"));
  await dialog.findByText(/db/);
  expect(dialog.getByRole("button", { name: de.common.stackUpdates.update }).hasAttribute("disabled")).toBe(true);
});

// The header hides its update action only when every service with an update is refused.
function HeaderDecision() {
  const e = useStackEligibility("direct~project");
  return <p>{e.checking || !e.isSuccess || !e.updatable ? "checking" : e.blocksAll ? "blocked" : "offered"}</p>;
}
it.each([["app", "offered"], ["db", "blocked"]] as const)("header decision with a binding on %s is %s", async (bound, decision) => {
  const f = await stackFixture(bound);
  renderWithApp(<HeaderDecision />, f.adapter);
  const shown = await screen.findByText(/^(offered|blocked)$/);
  expect(shown.textContent).toBe(decision);
});

it("keeps a manual check usable after an unknown outcome and says it did not finish", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  let calls = 0;
  const check = async () => { calls++; if (calls === 1) throw new AdapterOutcomeError({ kind: "error", code: "runtime.admission-unknown" }); return { hasUpdate: false }; };
  renderWithApp(<CheckUpdatesButton owner="example" check={check} />, f.adapter);
  const button = await screen.findByRole("button", { name: de.containers.updateCheck.check });
  await userEvent.click(button);
  await screen.findByText(de.containers.updateCheck.unfinished);
  expect(button.hasAttribute("disabled")).toBe(false);
  await userEvent.click(button);
  await waitFor(() => expect(calls).toBe(2));
});

// A prediction belongs to the current successful read only.
it("drops a predicted refusal when the eligibility refetch fails", async () => {
  const f = updateFixture(); await f.adapter.getCurrentUser();
  const host = (await createDemoAdapter().listHosts())[0]!;
  host.id = "direct"; host.componentVersion = { ...host.componentVersion, agentContractVersion: 23 } as NonNullable<typeof host.componentVersion>;
  f.adapter.listHosts = async () => [host];
  let failed = false;
  f.adapter.getUpdateEligibility = async () => { if (failed) throw new Error("failed read"); return { project: { scope: "project", reason: "unparsable" }, services: [] }; };
  function Probe() {
    const e = useStackEligibility("direct~project");
    return <><p>{e.isError ? "failed" : e.isSuccess ? "ready" : "pending"}</p><p>{e.blocksAll ? "blocked" : "offered"}</p><button onClick={() => { failed = true; void e.refetch(); }}>refetch</button><StackBindingNotice stackId="direct~project" /></>;
  }
  renderWithApp(<Probe />, f.adapter);
  await screen.findByText("ready");
  expect(screen.getByText("blocked").textContent).toBe("blocked");
  await userEvent.click(screen.getByRole("button", { name: "refetch" }));
  await screen.findByText("failed");
  expect(screen.queryByText("blocked") === null).toBe(true);
  expect(screen.queryByRole("status") === null).toBe(true);
});
