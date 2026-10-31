// Source: docklet-next/issues/318
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original host edit and removal gesture coverage. M14 host maintenance and availability.

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
it("H6 shows unknown usage on the card and states in German that the agent keeps running", async () => {
  setAppLocale("de");
  renderWithApp(<HostCard host={{ ...host, status: "offline" }} system={undefined} stacks={undefined} containers={undefined} allIds={[host.id]} />, createDemoAdapter());
  expect(screen.queryByText("Stacks und Container: unbekannt") !== null).toBe(true);
  expect(screen.queryByText(/0 Stacks/) === null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Entfernen" }));
  expect((await screen.findByRole("alertdialog")).textContent?.includes("Der Agent läuft weiter, bis du ihn auf dem Host stoppst.")).toBe(true);
});
it("H1 ignores a save response after the dialog closes", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  let release!: (host: import("@/domain/model").Host) => void;
  vi.spyOn(adapter, "updateHost").mockImplementation(() => new Promise(resolve => { release = resolve; }));
  const changed = vi.fn();
  const { rerender } = renderWithApp(<HostSettingsDialog host={host} component={undefined} open onOpenChange={changed} />, adapter);
  const name = screen.getByLabelText("Display name"); await userEvent.clear(name); await userEvent.type(name, "Renamed");
  await userEvent.click(screen.getByRole("button", { name: "Save" }));
  await waitFor(() => expect(release).toBeTypeOf("function"));
  // Unmount represents navigation; the old response must not close a later dialog.
  rerender(<div />);
  const { act } = await import("@testing-library/react");
  await act(async () => { release({ ...host, name: "Renamed" }); });
  expect(changed).not.toHaveBeenCalled();
});
