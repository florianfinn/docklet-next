// Source: docklet-next/issues/307
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original availability consumer and typed refusal gestures.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { toast } from "sonner";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapHost } from "@/data/http/hosts";
import { operationReasonKey } from "@/data/operation-reason";
import { SettingsProblemError, settingsProblem, settingsErrorMessage } from "@/data/settings-error";
import { isHostAvailable, hostTone } from "@/domain/hosts";
import { renderWithApp } from "@/test/render";
import { AutoHealSection } from "@/features/settings/sections/AutoHealSection";
import { HostCard } from "./HostCard";

const host = mapHost({ id: "atlas", name: "Example", color: null, apiUrl: "http://agent.example.com", enabled: true, status: "standby", isEdge: true });
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });
it("H7 preserves neutral standby, available for waking actions; offline and refusals stay unavailable", () => {
  expect(host.status).toBe("standby"); expect(hostTone(host)).toBe("gray"); expect(isHostAvailable(host)).toBe(true);
  for (const status of ["offline", "pending", "token-rejected", "update-required"] as const) expect(isHostAvailable({ ...host, status })).toBe(false);
  expect(mapHost({ id: "example", color: null, apiUrl: "", enabled: false, status: "standby", isEdge: true }).status).toBe("offline");
});
it("H7 loads Auto Heal for standby, retains offline unavailability", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  const hosts = await adapter.listHosts();
  vi.spyOn(adapter, "listHosts").mockResolvedValue(hosts.map(h => h.id === "atlas" ? { ...h, status: "standby" } : h));
  const read = vi.spyOn(adapter, "getAutoHealSettings");
  renderWithApp(<AutoHealSection />, adapter);
  expect(await screen.findByRole("button", { name: "Edit: Atlas" }) !== null).toBe(true);
  expect(read.mock.calls.some(([id]) => id === "atlas")).toBe(true);
  expect(read.mock.calls.some(([id]) => id === "cedar")).toBe(false);
});
it("H2 reports a standby-opened check from an actual gesture", async () => {
  setAppLocale("en-US"); const adapter = createDemoAdapter();
  vi.spyOn(adapter, "reconnectHost").mockResolvedValue({ status: "standby-opened" });
  const shown = vi.spyOn(toast, "success").mockImplementation(() => "example");
  renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, adapter);
  expect(screen.getByText("Standby – connects on demand") !== null).toBe(true);
  await userEvent.click(screen.getByRole("button", { name: "Check connection" }));
  await waitFor(() => expect(shown).toHaveBeenCalledWith("Standby tunnel opened – connection verified"));
});
it("H7 maps UI-disabled refusals while heartbeat-stale remains internal", () => {
  const problem = settingsProblem({ code: "agent.ui-configuration-disabled", status: 502 }, 502);
  expect(problem instanceof SettingsProblemError).toBe(true);
  expect(settingsErrorMessage(problem, ((key: string) => key) as never)).toBe("hosts.errors.uiConfigurationDisabled");
  expect(operationReasonKey("agent.ui-configuration-disabled")).toBe("uiConfigurationDisabled");
  expect(operationReasonKey("edge.heartbeat-stale")).toBeUndefined();
});
it.each(["en-US", "de"] as const)("H7 shows the typed environment-managed message in %s", async locale => {
  setAppLocale(locale); const adapter = createDemoAdapter();
  vi.spyOn(adapter, "reconnectHost").mockRejectedValue(new SettingsProblemError("agent.ui-configuration-disabled", 502));
  const shown = vi.spyOn(toast, "error").mockImplementation(() => "example");
  renderWithApp(<HostCard host={host} system={undefined} stacks={2} containers={3} allIds={[host.id]} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Verbindung prüfen" : "Check connection" }));
  expect(shown).toHaveBeenCalledWith(locale === "de" ? "Einstellungen dieses Hosts werden über seine Umgebung verwaltet" : "Settings on this host are managed through its environment");
});
