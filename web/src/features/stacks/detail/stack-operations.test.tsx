// Source: docklet-next/issues/317
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original W5/W6/W7 service and stopped-definition UI gestures.

import { ActionsMenu } from "@/shared/components/action-menu";
import { useStackMenu } from "../overview/stack-menu";
import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import { renderStackDetail } from "@/data/message-code";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { mapStack, type ProjectRow } from "@/data/http/inventory-mapping";
import type { StackDetail } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { ServiceRestartDialog } from "@/shared/components/service-restart";
import { GeneralSection } from "@/features/settings/sections/GeneralSection";
import { AttentionServiceRestart } from "@/features/dashboard/widgets/AttentionServiceRestart";
import { ServicesCard } from "./ServicesCard";
import { StackHeader } from "./StackHeader";

it("W7 maps not-created services to stopped and offers no container links or logs", async () => {
  const adapter = createDemoAdapter();
  const original = renderStackDetail(await adapter.getStack("media"), "de");
  const stack = mapStack({ id: "media", name: "Media", status: "stopped", runtimeServices: [{ name: "api", image: "example/app:1", status: "not-created" }] } as unknown as ProjectRow, "0", [], { stale: false });
  expect(stack.services[0]).toMatchObject({ state: "stopped", containersRemoved: true });
  expect(stack.services[0]!.containerId).toBeUndefined();
  const rendered = renderWithApp(<ServicesCard stack={{ ...original, ...stack }} host={undefined} />, adapter);
  expect(screen.getByText("Container entfernt") !== null).toBe(true);
  expect(screen.getByText("Gestoppt") !== null).toBe(true);
  expect(rendered.container.querySelectorAll('a[href*="/containers"]').length).toBe(0);
  expect(screen.queryByRole("button", { name: /Logs/ }) === null).toBe(true);
});

it("W7 retains an invalid-definition stack and shows the repair hint", async () => {
  const adapter = createDemoAdapter(); const original = renderStackDetail(await adapter.getStack("media"), "de");
  const stack = mapStack({ id: "media", name: "Media", status: "stopped", definitionHint: "definition-invalid", runtimeServices: [] } as unknown as ProjectRow, "0", [], { stale: false });
  expect(stack.services).toEqual([]);
  renderWithApp(<StackHeader stack={{ ...original, ...stack }} host={undefined} />, adapter);
  expect(screen.getByText(/Die Compose-Definition kann nicht aufgelöst/) !== null).toBe(true);
});

it("W5 services and attention gestures restart only the selected service through the scoped capability", async () => {
  const adapter = createDemoAdapter(); const stack = renderStackDetail(await adapter.getStack("media"), "de");
  const restart = vi.spyOn(adapter, "restartService").mockResolvedValue("service-run");
  const capability = vi.spyOn(adapter, "getCapability");
  const rendered = renderWithApp(<><ServicesCard stack={stack} host={undefined} /><ServiceRestartDialog /></>, adapter);
  const service = stack.services[0]!.name;
  await userEvent.click(await screen.findByRole("button", { name: `${service} neu starten` }));
  await userEvent.click(await screen.findByRole("button", { name: "Nur diesen Service neu starten" }));
  await waitFor(() => expect(restart).toHaveBeenCalledExactlyOnceWith(stack.id, service));
  expect(capability).toHaveBeenCalledWith("restartService", { kind: "stack", id: stack.id });
  rendered.unmount();
  const attentionAdapter = createDemoAdapter();
  const attentionRestart = vi.spyOn(attentionAdapter, "restartService").mockResolvedValue("attention-run");
  renderWithApp(<><AttentionServiceRestart item={{ id: "attention", kind: "unhealthy", tone: "err", weight: 1, hostId: stack.hostId, stackId: stack.id, service, values: { stack: stack.name } }} /><ServiceRestartDialog /></>, attentionAdapter);
  await userEvent.click(await screen.findByRole("button", { name: "Neustart" }));
  await userEvent.click(await screen.findByRole("button", { name: "Nur diesen Service neu starten" }));
  await waitFor(() => expect(attentionRestart).toHaveBeenCalledExactlyOnceWith(stack.id, service));
});

it("W6 saves the hub stop mode through settings with a real Select gesture", async () => {
  const adapter = createDemoAdapter(); const settings = await adapter.getSettings();
  const save = vi.spyOn(adapter, "saveSettings").mockResolvedValue();
  renderWithApp(<GeneralSection settings={settings} />, adapter);
  const select = await screen.findByRole("combobox", { name: "Stopp-Modus für Stacks" });
  select.focus(); await userEvent.keyboard("{ArrowDown}");
  await userEvent.click(await screen.findByRole("option", { name: "Stoppen und Container behalten" }));
  await userEvent.click(screen.getByRole("button", { name: "Speichern" }));
  await waitFor(() => expect(save).toHaveBeenCalledWith("general", expect.objectContaining({ stackStopMode: "stop" })));
});

it("W6 disables stack actions with a typed update-required reason for a v10 host", async () => {
  const adapter = createDemoAdapter(); const stack = renderStackDetail(await adapter.getStack("media"), "de");
  vi.spyOn(adapter, "listHosts").mockResolvedValue([{ ...(await adapter.listHosts())[0]!, id: stack.hostId, status: "update-required" }]);
  renderWithApp(<StackHeader stack={stack as StackDetail} host={undefined} />, adapter);
  await waitFor(() => expect((screen.getByRole("button", { name: "Stoppen" }) as HTMLButtonElement).disabled).toBe(true));
  expect(screen.getByRole("button", { name: "Stoppen" }).getAttribute("title")).toContain("Aktualisiere den Agenten");
});

function StackMenuFixture({ stack }: { stack: StackDetail }) {
  const groups = useStackMenu(stack, false);
  return <ActionsMenu groups={groups} label="Actions" />;
}
it("W6 exposes the other explicit stop action in the stack menu", async () => {
  const adapter = createDemoAdapter(); const stack = renderStackDetail(await adapter.getStack("media"), "de");
  const settings = await adapter.getConnectedSettings();
  vi.spyOn(adapter, "getConnectedSettings").mockResolvedValue({ ...settings, general: { ...settings.general, stackStopMode: "stop" } });
  adapter.getStackStopMode = async () => "stop";
  const run = vi.spyOn(adapter, "runStackCommand").mockResolvedValue("down-run");
  renderWithApp(<StackMenuFixture stack={stack} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Actions" }));
  await userEvent.click(await screen.findByRole("menuitem", { name: "Herunterfahren (Container entfernen)" }));
  await waitFor(() => expect(run).toHaveBeenCalledExactlyOnceWith(stack.id, "down"));
});

it("W6 fails closed when the hub stop mode cannot be read and offers both explicit actions", async () => {
  const adapter = createDemoAdapter(); const stack = renderStackDetail(await adapter.getStack("media"), "de");
  adapter.getStackStopMode = () => Promise.reject(new Error("settings.read.403"));
  const run = vi.spyOn(adapter, "runStackCommand").mockResolvedValue("stop-run");
  renderWithApp(<StackMenuFixture stack={stack} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Actions" }));
  const stop = await screen.findByRole("menuitem", { name: "Stoppen" });
  await waitFor(() => expect(stop.getAttribute("aria-disabled") === "true").toBe(true));
  expect(screen.queryByRole("menuitem", { name: "Herunterfahren (Container entfernen)" }) !== null).toBe(true);
  await userEvent.click(screen.getByRole("menuitem", { name: "Container stoppen" }));
  await waitFor(() => expect(run).toHaveBeenCalledExactlyOnceWith(stack.id, "stop-containers"));
});

it("W6 an unrelated settings failure does not block Stop", async () => {
  const adapter = createDemoAdapter(); const stack = renderStackDetail(await adapter.getStack("media"), "de");
  vi.spyOn(adapter, "getConnectedSettings").mockRejectedValue(new Error("settings.invalid.linkBase"));
  adapter.getStackStopMode = async () => "down";
  const run = vi.spyOn(adapter, "runStackCommand").mockResolvedValue("stop-run");
  renderWithApp(<StackMenuFixture stack={stack} />, adapter);
  await userEvent.click(screen.getByRole("button", { name: "Actions" }));
  const stop = await screen.findByRole("menuitem", { name: "Stoppen" });
  await waitFor(() => expect(stop.getAttribute("aria-disabled") === "true").toBe(false));
  await userEvent.click(stop);
  await waitFor(() => expect(run).toHaveBeenCalledExactlyOnceWith(stack.id, "stop"));
});
