// Source: docklet-next/docs/design/hub-backup.md
// Revision: 8f8c8f89bc6ebb7261862d0c35a6ddbc1513e9fa
// Retrieved: 2026-10-16
// License: Apache-2.0
// Deviations: Original UI state tests for issue #32.

import { fireEvent, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { NAV, visibleNavItems } from "@/app/shell/nav-items";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { DemoAdapterOptions } from "@/data/demo/options";
import { MAX_HUB_BACKUP_BYTES } from "@/domain/hub-backup";
import { renderRoutes } from "@/test/render";

import { deHubBackups } from "./messages/de";
import { enHubBackups } from "./messages/en";
import HubBackupsPage from "./HubBackupsPage";

function page(scenario: DemoAdapterOptions["hubBackupScenario"] = "list") {
  const adapter = createDemoAdapter({ hubBackupScenario: scenario });
  const result = renderRoutes([
    { path: "/hub-backups", element: <HubBackupsPage /> },
    { path: "/login", element: <p>Sign-in route</p> },
  ], ["/hub-backups"], adapter);
  return { adapter, ...result };
}

afterEach(() => vi.restoreAllMocks());

describe("Hub database backup page", () => {
  it("shows the empty state and creates a manual archive", async () => {
    const { adapter } = page("empty");
    expect(await screen.findByText(deHubBackups.emptyTitle)).toBeTruthy();
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.create }));
    await waitFor(() => expect(screen.getByText(deHubBackups.statusValues.running)).toBeTruthy());
    expect((await adapter.listHubBackups())[0]?.trigger).toBe("manual");
  });

  it("lists size, creation, trigger and status and keeps target copy hidden", async () => {
    page();
    expect(await screen.findByText("system-demo-1")).toBeTruthy();
    expect(screen.getByText("32 MB")).toBeTruthy();
    expect(screen.getByText(deHubBackups.triggerValues.manual)).toBeTruthy();
    expect(screen.getByText(deHubBackups.statusValues.succeeded)).toBeTruthy();
    expect(screen.getByRole("button", { name: deHubBackups.download })).toBeTruthy();
    expect(screen.queryByText(/Zielkopie|Backupziel/)).toBeNull();
  });

  it.each([["creating", "running"], ["queued", "queued"], ["cancelled", "cancelled"]] as const)("renders %s as %s", async (scenario, state) => {
    page(scenario);
    expect(await screen.findByText(deHubBackups.statusValues[state])).toBeTruthy();
  });

  it("shows a cancelled activity beyond the first twenty archives", async () => {
    const adapter = createDemoAdapter();
    const sample = (await adapter.listHubBackups())[0]!;
    adapter.listHubBackups = async () => Array.from({ length: 25 }, (_, index) => ({ ...sample, id: `system-${index + 1}`, activityId: `activity-${index + 1}`, status: "failed" }));
    adapter.getHubBackupActivityStatus = async (id) => id === "activity-25" ? "cancelled" : "failed";
    renderRoutes([{ path: "/hub-backups", element: <HubBackupsPage /> }], ["/hub-backups"], adapter);
    expect(await screen.findByText("system-25")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByText(deHubBackups.statusValues.cancelled).length).toBe(1));
  });

  it.each([["storage-unavailable", "storageUnavailable"], ["tools-unavailable", "toolsUnavailable"]] as const)("explains %s and disables writes", async (scenario, message) => {
    page(scenario);
    expect(await screen.findByRole("alert")).toHaveProperty("textContent", deHubBackups[message]);
    expect((screen.getByRole("button", { name: deHubBackups.create }) as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: deHubBackups.upload }) as HTMLButtonElement).disabled).toBe(true);
  });

  it("rejects an oversized upload and a wrong media type before calling the adapter", async () => {
    const { adapter } = page();
    const send = vi.spyOn(adapter, "uploadHubBackup");
    await screen.findByText("system-demo-1");
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.upload }));
    const input = screen.getByLabelText(deHubBackups.chooseFile);
    const large = new File(["x"], "large.dump", { type: "application/octet-stream" });
    Object.defineProperty(large, "size", { value: MAX_HUB_BACKUP_BYTES + 1 });
    fireEvent.change(input, { target: { files: [large] } });
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.uploadAction }));
    expect(screen.getByRole("alert").textContent).toBe(deHubBackups.errors.size);
    fireEvent.change(input, { target: { files: [new File(["x"], "notes.txt", { type: "text/plain" })] } });
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.uploadAction }));
    expect(screen.getByRole("alert").textContent).toBe(deHubBackups.errors.type);
    expect(send).not.toHaveBeenCalled();
  });

  it("requires the safety choice and exact archive ID before restore", async () => {
    const { adapter } = page();
    const restore = vi.spyOn(adapter, "restoreHubBackup");
    await screen.findByText("system-demo-1");
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.restore }));
    expect(screen.getByText(/meldet alle Benutzer ab/)).toBeTruthy();
    const confirm = screen.getByRole("button", { name: deHubBackups.restoreAction }) as HTMLButtonElement;
    expect(confirm.disabled).toBe(true);
    await userEvent.type(screen.getByLabelText(deHubBackups.restoreType), "system-demo-1");
    expect(confirm.disabled).toBe(true);
    await userEvent.click(screen.getByRole("checkbox", { name: deHubBackups.restoreSafety }));
    expect(confirm.disabled).toBe(false);
    expect(restore).not.toHaveBeenCalled();
    await userEvent.click(confirm);
    await waitFor(() => expect(restore).toHaveBeenCalledWith("system-demo-1"));
    expect(await screen.findByText(deHubBackups.maintenanceTitle)).toBeTruthy();
  });

  it("saves the schedule and keep count and uploads a supported archive with progress", async () => {
    const { adapter } = page();
    await screen.findByText("system-demo-1");
    const count = screen.getByLabelText(deHubBackups.keepCount);
    await userEvent.clear(count);
    await userEvent.type(count, "3");
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.save }));
    await waitFor(async () => expect((await adapter.getHubBackupPolicies())[0]?.retentionCount).toBe(3));
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.upload }));
    const file = new File(["PGDMP"], "trusted.dump", { type: "application/octet-stream" });
    fireEvent.change(screen.getByLabelText(deHubBackups.chooseFile), { target: { files: [file] } });
    await userEvent.click(screen.getByRole("button", { name: deHubBackups.uploadAction }));
    expect(await screen.findByText(deHubBackups.uploadProgress.replace("{percent}", "45"))).toBeTruthy();
    await waitFor(async () => expect((await adapter.listHubBackups()).some((run) => run.trigger === "uploaded")).toBe(true));
  });

  it("shows maintenance progress and redirects after sessions are revoked", async () => {
    const { adapter, router } = page("maintenance");
    expect(await screen.findByText(deHubBackups.maintenanceTitle)).toBeTruthy();
    expect(screen.getByText(deHubBackups.maintenancePhase.draining)).toBeTruthy();
    vi.spyOn(adapter, "getHubBackupMaintenance").mockResolvedValue({ active: false });
    await waitFor(() => expect(router.state.location.pathname).toBe("/login"), { timeout: 3500 });
  });

  it("hides the page and navigation from a non-admin", async () => {
    page("no-access");
    expect(await screen.findByText(deHubBackups.accessDenied)).toBeTruthy();
    expect(screen.queryByText(deHubBackups.archives)).toBeNull();
    const system = NAV.find((group) => group.key === "system")!;
    expect(visibleNavItems(system.items, false).some((item) => item.key === "hubBackups")).toBe(false);
    expect(visibleNavItems(system.items, true).some((item) => item.key === "hubBackups")).toBe(true);
  });

  it("keeps English and German catalogue keys and placeholders aligned", () => {
    const flatten = (value: unknown, prefix = ""): Record<string, string> => Object.fromEntries(Object.entries(value as Record<string, unknown>).flatMap(([key, next]) => typeof next === "string" ? [[`${prefix}${key}`, next]] : Object.entries(flatten(next, `${prefix}${key}.`))));
    const en = flatten(enHubBackups), de = flatten(deHubBackups);
    expect(Object.keys(de).sort()).toEqual(Object.keys(en).sort());
    for (const key of Object.keys(en)) expect([...de[key]!.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort(), key).toEqual([...en[key]!.matchAll(/\{(\w+)\}/g)].map((match) => match[1]).sort());
  });
});
