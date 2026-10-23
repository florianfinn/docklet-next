// Source: docklet-next/docs/design/network-management.md
// Revision: 2e2b3e5996afd98ee8edca0a8899947b62a2a339
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original resource-scoped UI and late network response tests for #204.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { CreateNetworkInput } from "@/data/api";
import { networkFixture, networkDraft, recordedNetworkResponses } from "@/data/http/network-commands.test-support";
import { renderWithApp } from "@/test/render";
import { CreateNetworkDialog } from "./CreateNetworkDialog";
import { NetworkTable } from "./NetworkTable";
import { TopologyDetails } from "./TopologyDetails";
import NetworksPage from "./NetworksPage";
import { useNetworkCommands } from "./use-network-commands";

afterEach(() => vi.restoreAllMocks());

const barrier = () => { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; };

describe.each([false, true])("HTTP network UI (Edge=%s)", (edge) => {
  it("keeps network actions available after an unavailable host-scoped remove query", async () => {
    const f = networkFixture(edge);
    expect((await f.adapter.getCapability("removeNetwork", { kind: "host", id: "host" })).available).toBe(false);
    const hosts = await f.adapter.listHosts(); const topology = await f.adapter.getTopology();
    const host = hosts.find((h) => h.id === "host")!;
    renderWithApp(<NetworkTable host={host} networks={topology.networks.filter((n) => n.hostId === "host")} topology={topology} containers={[]} focus={null} query="" onClearQuery={() => {}} onFocus={() => {}} onConnect={() => {}} onRemove={() => {}} />, f.adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Aktionen für example_net" }));
    await waitFor(() => expect(screen.getByRole("menuitem", { name: "Entfernen" }).getAttribute("aria-disabled") === "true").toBe(false));
    expect(screen.getByRole("menuitem", { name: /verbinden/i }).getAttribute("aria-disabled") === "true").toBe(false);
  });
  it("gates the page toolbar by the selected host and its network descriptors", async () => {
    const f = networkFixture(edge); f.state.deniedHosts.add("host");
    renderWithApp(<NetworksPage />, f.adapter);
    const create = await screen.findByRole("button", { name: "Netzwerk anlegen" });
    expect((create as HTMLButtonElement).disabled).toBe(true);
    expect((screen.getByRole("button", { name: "Container verbinden" }) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(screen.getByRole("radio", { name: "other" }));
    await waitFor(() => expect((screen.getByRole("button", { name: "Netzwerk anlegen" }) as HTMLButtonElement).disabled).toBe(false));
    await waitFor(() => expect((screen.getByRole("button", { name: "Container verbinden" }) as HTMLButtonElement).disabled).toBe(false));
    const refs = f.fetcher.mock.calls.map(([url]) => new URL(String(url), "https://hub.example.com")).filter((url) => url.pathname.endsWith("/capabilities")).map((url) => [url.searchParams.get("kind"), url.searchParams.get("id")]);
    expect(refs).toContainEqual(["host", "other"]);
    expect(refs).toContainEqual(["network", "other~net1"]);
  });
  it("gates detail remove, connect and disconnect with the network ref", async () => {
    const f = networkFixture(edge); const hosts = await f.adapter.listHosts(); const topology = await f.adapter.getTopology();
    topology.attachments.push({ networkId: "host~net1", containerId: "host~container1", ip: "192.0.2.7" });
    renderWithApp(<TopologyDetails selected="host~net1" topology={topology} containers={[]} hosts={new Map(hosts.map((h) => [h.id, h]))} onSelect={() => {}} expanded={new Set()} onToggleExpand={() => {}} onConnect={() => {}} onRemove={() => {}} />, f.adapter);
    await waitFor(() => expect((screen.getByRole("button", { name: "Container verbinden" }) as HTMLButtonElement).disabled).toBe(false));
    expect(await screen.findByRole("button", { name: /trennen/i }) !== null).toBe(true);
    const urls = f.fetcher.mock.calls.map(([url]) => String(url));
    expect(urls.some((url) => url.includes("kind=network") && url.includes("host%7Enet1"))).toBe(true);
  });
  it.each(["overlap", "outcome-unknown"])("shows a translated %s toast", async (code) => {
    const f = networkFixture(edge); const error = vi.spyOn(toast, "error");
    if (code === "overlap") f.state.response = { body: { ...recordedNetworkResponses["create-invalid"], code: "network.overlap" }, status: 409 };
    else f.state.failTransport = true;
    function Probe() { const cmd = useNetworkCommands("host"); return <button onClick={() => void cmd.create(networkDraft())}>Create</button>; }
    renderWithApp(<Probe />, f.adapter);
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    await waitFor(() => expect(error).toHaveBeenCalledWith(expect.stringMatching(code === "overlap" ? /[Üü]berschneid/ : /[Uu]nbekannt/)));
  });
  it.each(["host switch", "closed dialog"])("suppresses a late create result after %s", async (change) => {
    const f = networkFixture(edge); const hosts = await f.adapter.listHosts();
    const entered = barrier(); const release = barrier(); const created = vi.fn(); const success = vi.spyOn(toast, "success");
    let completion: Promise<unknown> | undefined;
    const adapter = { ...f.adapter, createNetwork: (input: CreateNetworkInput) => {
      completion = (async () => { entered.release(); await release.promise; return f.adapter.createNetwork(input); })();
      return completion as ReturnType<typeof f.adapter.createNetwork>;
    } };
    let changeScope!: () => void;
    function Harness() {
      const [host, setHost] = useState("host"); const [open, setOpen] = useState(true);
      changeScope = () => change === "host switch" ? setHost("other") : setOpen(false);
      return <CreateNetworkDialog key={host} open={open} onOpenChange={setOpen} hosts={hosts} networks={[]} defaultHostId={host} onCreated={created} />;
    }
    renderWithApp(<Harness />, adapter);
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    await userEvent.type(within(dialog).getByLabelText("Name"), "new_network");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Anlegen" }));
    await entered.promise;
    await act(async () => { changeScope(); });
    await act(async () => { release.release(); await completion; });
    expect(created).not.toHaveBeenCalled(); expect(success).not.toHaveBeenCalled();
    if (change === "closed dialog") expect(screen.queryByRole("dialog") === null).toBe(true);
    else expect(screen.getByRole("dialog").textContent?.includes("other")).toBe(true);
  });
});
