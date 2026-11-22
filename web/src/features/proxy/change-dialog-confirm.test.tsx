// Source: docklet-next/docs/design/everyday-use.md
// Revision: 9967b32a4dd337d1765a96b5bb27027a1161190b
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Original M35 #559 tests: the named action is the confirmation; risk acknowledgements still gate it.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { renderWithApp } from "@/test/render";
import { ChangeDialog, type ChangeAction } from "./ChangeDialog";
import type { Inventory, Route } from "./types";

const t = catalogs.de.proxyContract.ui;
const route = { id: "route-1", service: "app", upstreams: ["http://app.example.com:8080"], dependants: ["route-1"] } as unknown as Route;
const actions: [ChangeAction, string][] = [
  [{ kind: "delete-route", target: "routes/app.yml", route }, t.delete],
  [{ kind: "insert-import", target: "Caddyfile" }, t.insertImport],
  [{ kind: "restore-owned", target: "routes/app.yml" }, t.restore],
  [{ kind: "edit-service", target: "routes/app.yml", route }, t.saveService],
];
const inventory = (risk: Partial<Inventory> = {}) => ({ importLine: "import routes/*", eligibility: { writable: true }, ...risk }) as unknown as Inventory;

it("each change submits with one click on its named action when no risk applies", async () => {
  for (const [action, label] of actions) {
    const submit = vi.fn();
    const view = renderWithApp(<ChangeDialog action={action} inventory={inventory()} busy={false} submit={submit} close={() => {}} />);
    const dialog = within(await screen.findByRole("dialog"));
    expect(dialog.queryByRole("checkbox") === null).toBe(true);
    await userEvent.click(dialog.getByRole("button", { name: label }));
    expect(submit).toHaveBeenCalledOnce();
    view.unmount();
  }
});

it("a required risk acknowledgement still holds the action until it is checked", async () => {
  for (const [risk, ack] of [[{ adminOff: true }, t.restartAck], [{ runningDiverged: true }, t.replaceAck]] as const) {
    const submit = vi.fn();
    const view = renderWithApp(<ChangeDialog action={actions[0]![0]} inventory={inventory(risk)} busy={false} submit={submit} close={() => {}} />);
    const dialog = within(await screen.findByRole("dialog"));
    const send = dialog.getByRole("button", { name: t.delete });
    expect(send.hasAttribute("disabled")).toBe(true);
    await userEvent.click(dialog.getByRole("checkbox", { name: ack }));
    expect(send.hasAttribute("disabled")).toBe(false);
    await userEvent.click(send);
    expect(submit).toHaveBeenCalledOnce();
    view.unmount();
  }
});
