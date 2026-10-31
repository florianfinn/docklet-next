// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original host edit and removal gesture coverage.

import { screen, within, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { AdapterOutcomeError } from "@/data/capability";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapHost } from "@/data/http/hosts";
import { defaultHostColor } from "@/domain/hosts";
import { renderWithApp } from "@/test/render";
import { HostSettingsDialog } from "./HostSettingsDialog";
import { HostCard } from "./HostCard";

const host = mapHost({ id: "atlas", name: "Example", color: "#112233", apiUrl: "http://agent.example.com", enabled: true, status: "online", isEdge: false });
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });
it.each(["host.name-invalid", "host.name-taken"])("H1 localizes a %s save refusal", async code => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  vi.spyOn(adapter, "updateHost").mockRejectedValue(new AdapterOutcomeError({ kind: "error", code }));
  renderWithApp(<HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} />, adapter);
  const name = screen.getByLabelText("Display name"); await userEvent.clear(name); await userEvent.type(name, "Renamed");
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  expect((await screen.findByRole("alert")).textContent).toBe(code === "host.name-taken" ? "A host with this name already exists." : "The name is empty or too long.");
});
it("H1 resets the colour through a real button and a null write", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  const save = vi.spyOn(adapter, "updateHost").mockResolvedValue({ ...host, color: defaultHostColor(host.id) });
  renderWithApp(<HostSettingsDialog host={host} component={undefined} open onOpenChange={() => {}} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Reset to default color" }));
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith(host.id, { name: host.name, color: null }));
});
it.each(["online", "offline"] as const)("H6 shows honest workload counts for %s and preserves workloads warning", async status => {
  setAppLocale("en-US");
  renderWithApp(<HostCard host={{ ...host, status }} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, createDemoAdapter());
  await userEvent.click(screen.getByRole("button", { name: "Remove" }));
  const dialog = await screen.findByRole("alertdialog");
  expect(dialog.textContent).toContain(status === "online" ? "Stacks: 2 · Containers: 3." : "Stacks: unknown · Containers: unknown.");
  expect(dialog.textContent).toContain("Containers and stacks keep running");
  expect(within(dialog).getByRole("button", { name: "Remove host" }) !== null).toBe(true);
});
