// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original real xterm and Radix gesture, reconnect and lifetime tests. M20 #360: shell readiness before Connected.

import { useState } from "react";
import { act, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { ShellEvent, ShellOptions } from "@/data/shell";
import { renderRoutes } from "@/test/render";
import ContainerPage from "./ContainerPage";
import { ContainerTerminalTabs } from "./ContainerTerminalTabs";

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

it("refreshes the shell descriptor when an offline-labelled host becomes standby", async () => {
  const user = userEvent.setup(), adapter = createDemoAdapter();
  const c = (await adapter.listContainers()).find((row) => row.id === "c-uptime-kuma")!;
  const host = (await adapter.listHosts()).find((row) => row.id === c.hostId)!;
  let eligible = false;
  const original = adapter.getCapability;
  const capability = vi.fn(async (...args: Parameters<typeof original>) => args[0] === "openContainerShell" ? { operationId: args[0], resourceRef: args[1]!, available: eligible, ...(eligible ? {} : { disabledReasonCode: "agent.unavailable" }) } : original(...args));
  adapter.getCapability = capability;
  function HostObservation() {
    const [seen, setSeen] = useState("2026-01-01T00:00:00Z");
    return <><button onClick={() => { eligible = true; setSeen("2026-01-01T00:00:01Z"); }}>Observe host</button><ContainerTerminalTabs c={c} host={{ ...host, status: "offline", lastSeen: seen }}><div>Logs</div></ContainerTerminalTabs></>;
  }
  renderRoutes([{ path: "/", element: <HostObservation /> }], ["/"], adapter);
  const tab = await screen.findByRole("tab", { name: "Terminal" });
  await screen.findByText(/Der Host ist nicht verfügbar/);
  expect(tab.hasAttribute("disabled")).toBe(true);
  await user.click(screen.getByRole("button", { name: "Observe host" }));
  expect(capability.mock.calls.filter(([operation]) => operation === "openContainerShell").length).toBe(2);
  await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false));
});

it("shows Connected and takes keyboard input only after the shell's first output, so its cursor reply precedes typed bytes", async () => {
  const user = userEvent.setup(), adapter = createDemoAdapter();
  const sent: string[] = [];
  let push: (event: ShellEvent) => void = () => {};
  adapter.openContainerShell = (_id, _query, listener) => { push = listener; return { send: (data) => { sent.push(data); }, resize: () => {}, close: () => {} }; };
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  const tab = await screen.findByRole("tab", { name: "Terminal" }); await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false)); await user.click(tab);
  const input = await screen.findByRole("textbox", { name: "Container-Terminaleingabe" });
  push({ type: "live" });
  await user.type(input, "early");
  expect(screen.queryByText("Verbunden") === null).toBe(true);
  // BusyBox ash prints its prompt and asks for the cursor position before reading a line.
  push({ type: "output", data: new TextEncoder().encode("/ # \u001b[6n") });
  await screen.findByText("Verbunden");
  await user.type(input, "echo ok{Enter}");
  const reply = sent[0] ?? "";
  expect(reply.startsWith("\u001b[") && /^\[\d+;\d+R$/.test(reply.slice(1))).toBe(true);
  expect(sent.slice(1).join("")).toBe("echo ok\r");
});

it("shows Connected after one second when the shell prints nothing", async () => {
  const user = userEvent.setup(), adapter = createDemoAdapter();
  let push: (event: ShellEvent) => void = () => {};
  adapter.openContainerShell = (_id, _query, listener) => { push = listener; return { send: () => {}, resize: () => {}, close: () => {} }; };
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  const tab = await screen.findByRole("tab", { name: "Terminal" }); await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false)); await user.click(tab);
  await screen.findByRole("textbox", { name: "Container-Terminaleingabe" });
  push({ type: "live" });
  expect(screen.queryByText("Verbunden") === null).toBe(true);
  await act(() => new Promise((resolve) => setTimeout(resolve, 1_200)));
  expect(screen.queryByText("Verbunden") !== null).toBe(true);
});

async function earlySession() {
  const user = userEvent.setup(), adapter = createDemoAdapter(), sent: string[] = [];
  let push: (event: ShellEvent) => void = () => {};
  adapter.openContainerShell = (_id, _query, listener) => { push = listener; listener({ type: "connecting" }); return { send: (data) => { sent.push(data); }, resize: () => {}, close: () => {} }; };
  renderRoutes([{ path: "/containers/:containerId", element: <ContainerPage /> }], ["/containers/c-uptime-kuma"], adapter);
  const tab = await screen.findByRole("tab", { name: "Terminal" }); await waitFor(() => expect(tab.hasAttribute("disabled")).toBe(false)); await user.click(tab);
  const input = await screen.findByRole("textbox", { name: "Container-Terminaleingabe" }) as HTMLTextAreaElement;
  act(() => push({ type: "live" }));
  return { user, input, sent, push };
}

it("holds pasted and composed text until the shell is ready, then accepts both", async () => {
  const f = await earlySession();
  f.input.focus(); await f.user.paste("echo early\n");
  f.input.dispatchEvent(new CompositionEvent("compositionstart", { bubbles: true }));
  f.input.value = "早";
  f.input.dispatchEvent(new CompositionEvent("compositionupdate", { bubbles: true, data: "早" }));
  f.input.dispatchEvent(new CompositionEvent("compositionend", { bubbles: true, data: "早" }));
  await act(() => new Promise((resolve) => setTimeout(resolve, 20)));
  expect(f.sent.length).toBe(0);
  act(() => f.push({ type: "output", data: new TextEncoder().encode("/ # ") }));
  await screen.findByText("Verbunden");
  f.input.focus(); await f.user.paste("echo ok");
  expect(f.sent.join("")).toBe("echo ok");
});
