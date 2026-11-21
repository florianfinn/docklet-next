// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import { fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { describe, expect, it } from "vitest";
import { Operations } from "./Operations";
import { fixture, operation, Providers, changedHash, hash } from "./proxy.test-support";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { instance } from "./proxy.test-support";

const states = [
  ["pending", "Pending", "info"],
  ["verified", "Verified", "ok"],
  ["applied-unverified", "Applied, not verified", "warn"],
  ["rejected", "Rejected", "err"],
  ["restored", "Restored", "warn"],
  ["recovery-required", "Recovery required", "err"],
  ["released", "Released", "info"],
  ["not-applied", "Not applied", "info"],
] as const;
describe("operation receipts through HTTP decoder to production renderer", () => {
  it.each(states)("renders %s with exact tone, evidence and inert diagnostic text", async (state, label, tone) => {
    const f = fixture();
    f.state.operations = [
      operation({
        state,
        reason: "no-observable-difference",
        evidence: "configuration",
        current: { present: true, hash: changedHash },
        diagnostic: { path: "routes/app.yml", line: 3, detail: '<img src=x onerror="alert(1)">\nsecond line' },
      }),
    ];
    const rows = await f.api.operations(instance.id);
    const view = render(
      <Providers>
        <Operations rows={rows} busy={false} release={async () => {}} />
      </Providers>,
    );
    expect(screen.getByText(label).getAttribute("data-tone")).toBe(tone);
    expect(screen.getByText("Configuration evidence").textContent).toBe("Configuration evidence");
    expect(screen.getByText("No observable difference.").textContent).toBe("No observable difference.");
    expect(screen.getByText("routes/app.yml:3").textContent).toBe("routes/app.yml:3");
    const diagnostic = screen.getByLabelText("Diagnostic");
    expect(diagnostic.textContent?.includes('<img src=x onerror="alert(1)">\nsecond line')).toBe(true);
    expect(view.container.querySelector("img") === null).toBe(true);
    expect(screen.queryByRole("button", { name: "Release" }) !== null).toBe(state === "recovery-required");
    if (state === "pending") expect(screen.getByText("Checking").textContent).toBe("Checking");
  });
  it.each(["routing", "load"] as const)("renders %s evidence", async evidence => {
    const f = fixture();
    f.state.operations = [operation({ evidence })];
    const rows = await f.api.operations(instance.id);
    render(
      <Providers>
        <Operations rows={rows} busy={false} release={async () => {}} />
      </Providers>,
    );
    expect(
      (screen.getByText(evidence === "routing" ? "Routing evidence" : "Load evidence").textContent?.length ?? 0) > 0,
    ).toBe(true);
  });
  it("requires release confirmation and sends the displayed current presence/hash", async () => {
    const f = fixture();
    f.state.operations = [operation({ state: "recovery-required", current: { present: true, hash } })];
    f.state.next = operation({ state: "recovery-required", current: { present: true, hash: changedHash } });
    render(
      <Providers>
        <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
      </Providers>,
    );
    const user = userEvent.setup();
    await user.click(await screen.findByRole("button", { name: "Release" }));
    const dialog = screen.getByRole("dialog");
    expect(dialog.textContent?.includes(`Present: ${changedHash}`)).toBe(true);
    const submit = within(dialog).getByRole("button", { name: "Release" });
    expect(submit.hasAttribute("disabled")).toBe(true);
    fireEvent.click(submit);
    expect(f.writes.filter(w => w.path.endsWith("/release")).length).toBe(0);
    await user.click(within(dialog).getByRole("checkbox", { name: "I confirm the displayed current target state." }));
    await user.click(submit);
    await waitFor(() => expect(f.writes.filter(w => w.path.endsWith("/release")).length).toBe(1));
    expect(f.writes.find(w => w.path.endsWith("/release"))?.body).toEqual({
      confirmed: { present: true, hash: changedHash },
    });
    await screen.findByText("Released");
    expect(screen.getByRole("button", { name: "Add route" }).hasAttribute("disabled")).toBe(false);
  });
  it("keeps recovery without a current target unreleasable", async () => {
    const f = fixture();
    f.state.operations = [operation({ state: "recovery-required", current: undefined })];
    render(
      <Providers>
        <Operations rows={await f.api.operations(instance.id)} busy={false} release={async () => {}} />
      </Providers>,
    );
    expect(screen.getByRole("button", { name: "Release" }).hasAttribute("disabled")).toBe(true);
  });
});
