// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Adopted from the M27 closing review (C-B1, C-B4, C-N2) counterexamples.

import { fireEvent, render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it } from "vitest";
import { InstanceWorkspace } from "./InstanceWorkspace";
import { fixture, instance, Providers, route } from "./proxy.test-support";
function show(f: ReturnType<typeof fixture>) {
  render(
    <Providers>
      <InstanceWorkspace instance={instance} api={f.api} updated={() => {}} removed={() => {}} />
    </Providers>,
  );
}
async function edit() {
  const user = userEvent.setup();
  await user.click(await screen.findByRole("button", { name: "Actions for app" }));
  await user.click(await screen.findByRole("menuitem", { name: "Edit route" }));
  return user;
}
const writes = (f: ReturnType<typeof fixture>) => f.writes.filter(w => w.path.endsWith("/operations"));
it("reviewer preserves an existing empty middleware reference on rename", async () => {
  const f = fixture();
  f.state.read.routes![0] = { ...route, references: [{ name: "", kind: "unresolved" }] };
  show(f);
  const user = await edit();
  fireEvent.change(screen.getByLabelText("Name", { selector: "input" }), { target: { value: "renamed" } });
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes(f).length).toBe(1));
  expect("middlewares" in (writes(f)[0]!.body as { change: object }).change).toBe(false);
});
it("reviewer can select the actual resolver named preserve", async () => {
  const f = fixture();
  f.state.inventory.resolvers = ["preserve"];
  show(f);
  const user = await edit();
  screen.getByRole("combobox", { name: "TLS resolver" }).focus();
  await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: /^preserve$/ }));
  await user.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(writes(f).length).toBe(1));
  expect((writes(f)[0]!.body as { change: { resolver?: string } }).change.resolver).toBe("preserve");
});
it("reviewer can open middleware selection when a readable definition has an empty name", async () => {
  const f = fixture();
  f.state.read.middlewares = [{ name: "", kind: "local", file: route.file }];
  show(f);
  const user = await edit();
  screen.getByRole("combobox", { name: "Existing middleware" }).focus();
  await user.keyboard("{ArrowDown}");
  await screen.findByRole("option", { name: "headers" });
  // The empty definition name is never offered as an option.
  expect(screen.getAllByRole("option").every(o => (o.textContent ?? "") !== "")).toBe(true);
});

