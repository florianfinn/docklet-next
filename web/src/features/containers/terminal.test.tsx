// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real xterm and Radix gesture, reconnect and lifetime tests.

import { screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { ShellEvent, ShellOptions } from "@/data/shell";
import { renderRoutes } from "@/test/render";
import ContainerPage from "./ContainerPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => { simulationStore.reset(); localStorage.clear(); });
function fixture() {
  const adapter = createDemoAdapter();
  const original = adapter.openContainerShell;
  const sessions: { query: ShellOptions; push: (event: ShellEvent) => void; send: ReturnType<typeof vi.fn>; close: ReturnType<typeof vi.fn> }[] = [];
  adapter.openContainerShell = (id, query, push) => {
    const session = original(id, query, push), send = vi.fn(session.send), close = vi.fn(session.close);
    sessions.push({ query, push, send, close }); return { ...session, send, close };
  };
  const view = renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  return { ...view, adapter, sessions };
}
it("uses real terminal typing, shell selection and explicit reconnect", async () => {
  const user = userEvent.setup(), f = fixture();
  const tab = await screen.findByRole("tab", { name: "Terminal" }); await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false)); await user.click(tab);
  const input = await screen.findByRole("textbox", { name: "Container-Terminaleingabe" }); await screen.findByText("Verbunden");
  await user.type(input, "echo example{Enter}");
  await waitFor(() => expect(f.sessions[0]!.send.mock.calls.map((call) => call[0]).join("")).toContain("echo example\r"));
  const select = screen.getByRole("combobox", { name: "Shell" }); select.focus(); await user.keyboard("{ArrowDown}");
  await user.click(await screen.findByRole("option", { name: "bash" }));
  await waitFor(() => expect(f.sessions.length).toBe(2)); expect(f.sessions[1]!.query.shell).toBe("bash"); expect(f.sessions[0]!.close).toHaveBeenCalledTimes(1);
  await user.click(screen.getByRole("button", { name: "Neu verbinden" })); await waitFor(() => expect(f.sessions.length).toBe(3)); expect(f.sessions[1]!.close).toHaveBeenCalledTimes(1);
  f.unmount(); expect(f.sessions[2]!.close).toHaveBeenCalledTimes(1);
});
it("keeps the exit authoritative and fences late callbacks after reconnect", async () => {
  const user = userEvent.setup(), f = fixture(); const tab = await screen.findByRole("tab", { name: "Terminal" }); await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false)); await user.click(tab);
  await screen.findByRole("textbox", { name: "Container-Terminaleingabe" }); await screen.findByText("Verbunden");
  f.sessions[0]!.push({ type: "exit", code: 17, reason: "container.shell-orphaned" });
  expect(await screen.findByText(/Ihr Prozess läuft möglicherweise/)).toBeTruthy();
  f.sessions[0]!.push({ type: "exit", code: 0, reason: "" });
  expect(screen.queryByText("Shell beendet (Exit-Code 0).")).toBe(null);
  await user.click(screen.getByRole("button", { name: "Neu verbinden" })); await waitFor(() => expect(f.sessions.length).toBe(2)); await screen.findByText("Verbunden");
  f.sessions[0]!.push({ type: "exit", code: 17, reason: "container.shell-unavailable" }); expect(screen.queryByText("Im Container fehlt /bin/sh.")).toBe(null); expect(screen.getByText("Verbunden") instanceof HTMLElement).toBe(true);
  f.unmount(); f.sessions[1]!.push({ type: "output", data: new TextEncoder().encode("late") });
});
it("disables stopped containers with a visible reason", async () => {
  const adapter = createDemoAdapter(), list = adapter.listContainers;
  adapter.listContainers = async (...args) => (await list(...args)).map((c) => c.id === "c-uptime-kuma" ? { ...c, state: "stopped" } : c);
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  expect((await screen.findByRole("tab", { name: "Terminal" })).hasAttribute("disabled")).toBe(true);
  expect(screen.getByText("Das Terminal ist verfügbar, wenn der Container läuft.") instanceof HTMLElement).toBe(true);
});
it("uses the host refusal reason from the shell descriptor", async () => {
  const adapter = createDemoAdapter(), capability = adapter.getCapability;
  adapter.getCapability = async (operation, ref) => operation === "openContainerShell" ? { operationId: operation, resourceRef: ref!, available: false, disabledReasonCode: "agent.unavailable" } : capability(operation, ref);
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  expect((await screen.findByRole("tab", { name: "Terminal" })).hasAttribute("disabled")).toBe(true);
  expect(await screen.findByText(/Der Host ist nicht verfügbar/)).toBeTruthy();
});
