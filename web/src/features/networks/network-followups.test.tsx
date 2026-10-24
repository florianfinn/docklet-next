// Source: docklet-next/issues/210
// Revision: 3cba55fd10c311e4a5b3361afabb8c93fca706a0
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original grant-lifetime, partial-read and dialog capability regressions.

import { useQueryClient, type QueryClient } from "@tanstack/react-query";
import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { CapabilityDescriptor, ResourceRef } from "@/data/capability";
import { useCapability } from "@/data/use-capability";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { networkContainer, networkFixture } from "@/data/http/network-commands.test-support";
import { topologyFixture } from "@/data/http/topology-resilience.test-support";
import type { Host } from "@/domain/model";
import { renderRoutes, renderWithApp } from "@/test/render";
import { ConnectNetworkDialog } from "./ConnectNetworkDialog";
import NetworksPage from "./NetworksPage";

const unavailable = "capability.unavailable";
const barrier = () => { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; };
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); });

function capabilityFixture(edge: boolean) {
  const f = networkFixture(edge);
  const denied = new Set<string>();
  const getCapability = async (operationId: string, resourceRef: ResourceRef = { kind: "hub", id: "0" }): Promise<CapabilityDescriptor> => ({ operationId, resourceRef, available: !denied.has(resourceRef.id), disabledReasonCode: denied.has(resourceRef.id) ? unavailable : undefined });
  return { ...f, denied, adapter: { ...f.adapter, getCapability } };
}

describe.each([false, true])("network follow-up UI (Edge=%s)", (edge) => {
  it("keeps the create draft and selected host across grant loss, host switch and identity change", async () => {
    setAppLocale("en-US");
    const f = capabilityFixture(edge); const create = vi.fn(f.adapter.createNetwork);
    let client!: QueryClient;
    function Probe() { client = useQueryClient(); const grant = useCapability("createNetwork", { kind: "host", id: "host" }); return <output data-testid="create-grant">{grant.available ? "available" : "unavailable"}</output>; }
    let version = 1;
    const subscribeChanges = f.adapter.subscribeChanges;
    const identityListeners = new Set<() => void>();
    registerIdentityLifetime(subscribeChanges, { version: () => version, active: () => true, listen: (listener) => { identityListeners.add(listener); return () => { identityListeners.delete(listener); }; } });
    const entered = barrier(), release = barrier(); let holdHosts = false;
    const adapter = { ...f.adapter, createNetwork: create, listHosts: async () => {
      if (holdHosts) { entered.release(); await release.promise; }
      return f.adapter.listHosts();
    } };
    const view = renderRoutes([{ path: "/networks", element: <><Probe /><NetworksPage /></> }], ["/networks?host=host"], adapter);
    const button = await screen.findByRole("button", { name: "Create network" });
    await waitFor(() => expect((button as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(button);
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    await userEvent.type(within(dialog).getByLabelText("Name"), "kept_draft");
    await userEvent.click(within(dialog).getByRole("button", { name: "Next" }));
    const submit = () => within(screen.getByRole("dialog")).getByRole("button", { name: "Create" }) as HTMLButtonElement;
    await waitFor(() => expect(submit().disabled).toBe(false));
    expect(submit().title).toBe("");
    f.denied.add("host");
    await act(async () => { await client.invalidateQueries({ queryKey: ["capability"] }); });
    await waitFor(() => expect(screen.getByTestId("create-grant").textContent).toBe("unavailable"));
    expect(submit().disabled).toBe(true);
    expect(screen.getByRole("dialog").textContent?.includes("kept_draft")).toBe(true);
    expect(screen.getByRole("dialog").textContent?.includes("capability could not be read")).toBe(true);
    await userEvent.click(submit()); expect(create).not.toHaveBeenCalled();
    await act(async () => { await view.router.navigate("/networks?host=other"); });
    expect(screen.getByRole("dialog").textContent?.includes("kept_draft")).toBe(true);
    expect(submit().disabled).toBe(true);
    f.denied.add("other"); holdHosts = true;
    act(() => { version++; identityListeners.forEach((listener) => listener()); });
    await entered.promise;
    expect(screen.queryByRole("dialog") !== null).toBe(true);
    expect(screen.getByRole("dialog").textContent?.includes("kept_draft")).toBe(true);
    expect(submit().disabled).toBe(true);
    await userEvent.click(submit()); expect(create).not.toHaveBeenCalled();
    await act(async () => { holdHosts = false; release.release(); });
    await waitFor(() => expect(client.isFetching()).toBe(0));
    expect(screen.getByRole("dialog").textContent?.includes("kept_draft")).toBe(true);
    expect(submit().disabled).toBe(true);
    await userEvent.click(submit()); expect(create).not.toHaveBeenCalled();
    f.denied.clear();
    await act(async () => { await client.invalidateQueries({ queryKey: ["capability"] }); });
    await waitFor(() => expect(submit().disabled).toBe(false));
    await userEvent.click(submit());
    await waitFor(() => expect(create).toHaveBeenCalledWith(expect.objectContaining({ hostId: "host", name: "kept_draft" })));
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
  });

  it("does not reopen a closed unavailable wizard when grants return", async () => {
    setAppLocale("en-US");
    const f = capabilityFixture(edge); let client!: QueryClient;
    function Probe() { client = useQueryClient(); const grant = useCapability("createNetwork", { kind: "host", id: "host" }); return <output data-testid="create-grant">{grant.available ? "available" : "unavailable"}</output>; }
    renderWithApp(<><Probe /><NetworksPage /></>, f.adapter);
    const create = await screen.findByRole("button", { name: "Create network" });
    await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(create);
    await screen.findByRole("dialog");
    f.denied.add("host"); f.denied.add("other");
    await act(async () => { await client.invalidateQueries({ queryKey: ["capability"] }); });
    expect(screen.queryByRole("dialog") !== null).toBe(true);
    await userEvent.keyboard("{Escape}");
    expect(screen.queryByRole("dialog") === null).toBe(true);
    f.denied.clear();
    await act(async () => { await client.invalidateQueries({ queryKey: ["capability"] }); });
    expect(screen.queryByRole("dialog") === null).toBe(true);
  });

  it("clears creation state after the host scope disappears and does not reopen on recovery", async () => {
    setAppLocale("en-US");
    const f = capabilityFixture(edge); const hosts = await f.adapter.listHosts();
    let shown = true, client!: QueryClient;
    function Probe() { client = useQueryClient(); return null; }
    renderWithApp(<><Probe /><NetworksPage /></>, { ...f.adapter, listHosts: async () => shown ? hosts : [] });
    const create = await screen.findByRole("button", { name: "Create network" });
    await waitFor(() => expect((create as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(create); await screen.findByRole("dialog");
    shown = false;
    await act(async () => { await client.invalidateQueries({ queryKey: ["hosts"] }); });
    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    shown = true;
    await act(async () => { await client.invalidateQueries({ queryKey: ["hosts"] }); });
    await waitFor(() => expect(screen.queryByRole("button", { name: "Create network" }) !== null).toBe(true));
    expect(screen.queryByRole("dialog") === null).toBe(true);
  });

  it.each(["offline", "missing capability"])("blocks an open connection wizard after %s", async (change) => {
    setAppLocale("en-US");
    const f = capabilityFixture(edge); f.state.containers = [structuredClone(networkContainer)];
    const topology = await f.adapter.getTopology();
    const containers = await f.adapter.listContainers();
    const host = (await f.adapter.listHosts()).find((h) => h.id === "host")!;
    const connect = vi.fn(f.adapter.connectNetwork);
    let lose!: () => void; let client!: QueryClient;
    function Harness() {
      client = useQueryClient();
      const grant = useCapability("connectNetwork", { kind: "network", id: "host~net1" });
      const [status, setStatus] = useState<Host["status"]>("online");
      useEffect(() => { lose = () => setStatus("offline"); }, []);
      return <><output data-testid="connect-grant">{grant.available ? "available" : "unavailable"}</output><ConnectNetworkDialog request={{ mode: "connect", networkId: "host~net1", containerId: "host~container1" }} onClose={() => {}} host={{ ...host, status }} topology={topology} containers={containers} /></>;
    }
    renderWithApp(<Harness />, { ...f.adapter, connectNetwork: connect });
    const dialog = await screen.findByRole("dialog");
    const review = within(dialog).getByRole("button", { name: "Review" }) as HTMLButtonElement;
    await waitFor(() => expect(review.disabled).toBe(false));
    await userEvent.click(review);
    if (change === "offline") act(() => lose());
    else { f.denied.add("host~net1"); await act(async () => { await client.invalidateQueries({ queryKey: ["capability"] }); }); }
    const confirm = within(dialog).getByRole("button", { name: "Connect" }) as HTMLButtonElement;
    if (change === "missing capability") await waitFor(() => expect(screen.getByTestId("connect-grant").textContent).toBe("unavailable"));
    expect(confirm.disabled).toBe(true);
    await userEvent.click(confirm); expect(connect).not.toHaveBeenCalled();
    await userEvent.click(within(dialog).getByRole("button", { name: "Back" }));
    expect((within(dialog).getByRole("button", { name: "Review" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

it.each(["en-US", "de"] as const)("shows one accurate partial-host notice in %s", async (locale) => {
  setAppLocale(locale);
  const f = topologyFixture(false); f.failures.add("/api/environments/second/networks/missing");
  const view = renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=second"], f.adapter);
  const text = locale === "de" ? "Bei second sind Netzwerklesevorgänge fehlgeschlagen." : "second: Some network reads failed";
  await waitFor(() => expect(view.container.textContent?.includes(text)).toBe(true));
  const notice = view.container.querySelector('[role="status"]');
  expect(notice?.textContent?.includes("second")).toBe(false);
  expect(view.container.querySelectorAll('p[aria-live="polite"]').length).toBe(1);
});

it.each(["en-US", "de"] as const)("keeps agent failure reasons and retained timestamps in %s", async (locale) => {
  setAppLocale(locale);
  const f = topologyFixture(false);
  f.state.reason = "agent.unavailable";
  const view = renderWithApp(<NetworksPage />, f.adapter);
  await screen.findByRole("radio", { name: "second" });
  expect(screen.getByRole("status").textContent?.includes(locale === "de" ? "Stand" : "as of")).toBe(true);
  expect(screen.getByRole("status").textContent?.includes("second")).toBe(true);
  expect(view.container.querySelectorAll('p[aria-live="polite"]').length).toBe(0);
});
