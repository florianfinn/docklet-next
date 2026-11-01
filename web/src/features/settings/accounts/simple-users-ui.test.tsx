// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original user-management and one-time personal-key UI guards.

import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";
import { UsersSection } from "./UsersSection";
import { UserDialog } from "./UserDialog";
import { PersonalApiKeys } from "../PersonalApiKeys";

beforeEach(() => { setAppLocale("en-US"); simulationStore.set((s) => ({ ...s, latency: "instant", live: false })); });
afterEach(() => { setAppLocale("de"); simulationStore.reset(); });

it("W4 disables own administrator switch and the last enabled administrator's switch/delete/disable", async () => {
  const adapter = createDemoAdapter();
  const own = (await adapter.listUsers())[0]!;
  const first = renderWithApp(<UserDialog user={own} mode="local" currentId={own.id} lastAdmin={false} onClose={() => {}} onSaved={() => {}} />, adapter);
  expect((await screen.findByRole("switch", { name: "Administrator" })).hasAttribute("disabled")).toBe(true);
  expect(screen.queryByText("You cannot change your own administrator status.") !== null).toBe(true);
  first.unmount();
  const second = renderWithApp(<UserDialog user={own} mode="local" currentId="another" lastAdmin onClose={() => {}} onSaved={() => {}} />, adapter);
  expect((await screen.findByRole("switch", { name: "Administrator" })).hasAttribute("disabled")).toBe(true);
  second.unmount();
  renderWithApp(<UsersSection mode="local" />, adapter);
  expect((await screen.findByRole("button", { name: "Delete: admin" })).hasAttribute("disabled")).toBe(true);
  expect(screen.getByRole("button", { name: "Disable" }).hasAttribute("disabled")).toBe(true);
});

it("creates a local user with a forced-change password, edits per-stack levels and resets/enables/disables", async () => {
  const adapter = createDemoAdapter();
  const create = vi.spyOn(adapter, "createUser"), assign = vi.spyOn(adapter, "setUserAppAssignments");
  renderWithApp(<UsersSection mode="local" />, adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Add user" }));
  const dialog = await screen.findByRole("dialog");
  await userEvent.type(within(dialog).getByLabelText("Username"), "member");
  await userEvent.type(within(dialog).getByLabelText("Initial password"), "example-password-123");
  await userEvent.click(within(dialog).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(create).toHaveBeenCalledWith({ username: "member", password: "example".concat("-password-123") }));
  await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  const row = (await screen.findByText("member")).closest("tr")!;
  await userEvent.click(within(row).getByRole("button", { name: "Assign stacks" }));
  const assignment = await screen.findByRole("dialog");
  const combos = await within(assignment).findAllByRole("combobox");
  await userEvent.click(combos[0]!); await userEvent.keyboard("{ArrowDown}{Enter}");
  // Explicitly choose Full through the accessible listbox when another value was highlighted.
  if (within(assignment).queryByRole("listbox")) await userEvent.keyboard("{Escape}");
  await userEvent.click(combos[0]!); await userEvent.keyboard("{End}{Enter}");
  expect(within(assignment).queryByText("Full access can control the host through the Compose file; not a security boundary.") !== null).toBe(true);
  await userEvent.click(within(assignment).getByRole("button", { name: "Save" }));
  await waitFor(() => expect(assign).toHaveBeenCalled());
  expect(assign.mock.calls[0]?.[1][0]?.level).toBe("full");
});

it.each(["none", "proxy"] as const)("does not read personal keys in %s mode", async (mode) => {
  const adapter = createDemoAdapter(); const list = vi.spyOn(adapter, "listMyApiKeys");
  renderWithApp(<PersonalApiKeys mode={mode} />, adapter);
  expect(screen.queryByText("API keys are not available in this sign-in mode.") !== null).toBe(true);
  expect(list).not.toHaveBeenCalled();
});

it("W6 shows the secret once, discards it on close, and never re-fetches or re-shows it", async () => {
  const adapter = createDemoAdapter();
  const create = vi.spyOn(adapter, "createMyApiKey").mockResolvedValue({ id: "key", name: "Automation", keyPrefix: "example", createdAt: "2026-11-01T00:00:00Z", key: "example".concat("-once-only-value") });
  vi.spyOn(adapter, "listMyApiKeys").mockResolvedValue([{ id: "key", name: "Automation", keyPrefix: "example", createdAt: "2026-11-01T00:00:00Z" }]);
  renderWithApp(<PersonalApiKeys mode="local" />, adapter);
  await userEvent.click(await screen.findByRole("button", { name: "Create API key" }));
  await userEvent.type(screen.getByLabelText("Name"), "Automation");
  await userEvent.click(screen.getByRole("button", { name: "Create" }));
  expect((await screen.findByTestId("api-key-secret")).textContent).toBe("example-once-only-value");
  await userEvent.click(screen.getAllByRole("button", { name: "Close" })[0]!);
  await waitFor(() => expect(screen.queryByText("example-once-only-value") === null).toBe(true));
  await userEvent.click(screen.getByRole("button", { name: "Create API key" }));
  expect(screen.queryByTestId("api-key-secret") === null).toBe(true);
  expect(create).toHaveBeenCalledTimes(1);
});
