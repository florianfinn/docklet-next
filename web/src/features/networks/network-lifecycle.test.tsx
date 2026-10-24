// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Accepted identity/refresh probes and page-level host ownership regression coverage.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";
import { toast } from "sonner";
import type { CreateNetworkInput } from "@/data/api";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { networkDto, networkFixture, networkDraft, recordedNetworkResponses } from "@/data/http/network-commands.test-support";
import { renderRoutes, renderWithApp } from "@/test/render";
import { useNetworkCommands } from "./use-network-commands";
import NetworksPage from "./NetworksPage";

const barrier = () => { let release!: () => void; const promise = new Promise<void>((resolve) => { release = resolve; }); return { promise, release }; };
afterEach(() => vi.restoreAllMocks());

describe.each([false, true])("network command ownership (Edge=%s)", (edge) => {
  it("drops a late create result after a browser identity change", async () => {
    const f = networkFixture(edge);
    const entered = barrier(); const release = barrier(); const success = vi.fn(); const error = vi.spyOn(toast, "error");
    let version = 1;
    const subscribeChanges = (listener: Parameters<typeof f.adapter.subscribeChanges>[0]) => f.adapter.subscribeChanges(listener);
    registerIdentityLifetime(subscribeChanges, { version: () => version, active: () => true, listen: () => () => {} });
    let completion: Promise<unknown> | undefined;
    const adapter = { ...f.adapter, subscribeChanges, createNetwork: (input: CreateNetworkInput) => {
      completion = (async () => { entered.release(); await release.promise; return f.adapter.createNetwork(input); })();
      return completion as ReturnType<typeof f.adapter.createNetwork>;
    } };
    function Probe() { const cmd = useNetworkCommands("host"); return <button onClick={() => void cmd.create(networkDraft()).then((result) => result && success(result))}>Create</button>; }
    renderWithApp(<Probe />, adapter);
    await userEvent.click(screen.getByRole("button", { name: "Create" }));
    await entered.promise;
    version = 2;
    await act(async () => { release.release(); await completion; });
    expect(success).not.toHaveBeenCalled(); expect(error).not.toHaveBeenCalled();
  });

  it.each(["failed", "uncertain"])("publishes refresh topics and clears cached topology after %s create", async (outcome) => {
    const f = networkFixture(edge);
    expect((await f.adapter.getTopology()).networks.some((n) => n.name === "discovered")).toBe(false);
    f.state.networks.push({ ...structuredClone(networkDto), id: "discovered", name: "discovered" });
    if (outcome === "uncertain") f.state.failTransport = true;
    else f.state.response = { body: { ...recordedNetworkResponses["create-invalid"], code: "network.overlap" }, status: 409 };
    const changed = vi.fn(); const stop = f.adapter.subscribeChanges(changed);
    try {
      await expect(f.adapter.createNetwork(networkDraft())).rejects.toMatchObject(outcome === "uncertain" ? { outcome: { code: "network.outcome-unknown" } } : { code: "overlap" });
      expect(changed).toHaveBeenCalledWith(["topology", "containers"]);
      expect((await f.adapter.getTopology()).networks.some((n) => n.id === "host~discovered")).toBe(true);
    } finally { stop(); }
  });

  it("drops a page-level late remove result after host switch without losing the new focus", async () => {
    const f = networkFixture(edge); const entered = barrier(); const release = barrier();
    const success = vi.spyOn(toast, "success"); let completion: Promise<void> | undefined;
    const adapter = { ...f.adapter, removeNetwork: (id: string) => {
      completion = (async () => { entered.release(); await release.promise; await f.adapter.removeNetwork(id); })();
      return completion;
    } };
    const { router } = renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=host&node=host~net1"], adapter);
    const remove = await screen.findByRole("button", { name: "Netzwerk entfernen" });
    await waitFor(() => expect((remove as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(remove);
    await userEvent.click(within(await screen.findByRole("alertdialog")).getByRole("button", { name: "Entfernen" }));
    await entered.promise;
    await act(async () => { await router.navigate("/networks?host=other&node=other~net1"); });
    expect(document.querySelector('button[role="radio"][aria-checked="true"]')?.textContent).toBe("other");
    await act(async () => { release.release(); await completion; });
    expect(success).not.toHaveBeenCalled();
    const params = new URLSearchParams(router.state.location.search);
    expect([params.get("host"), params.get("node")]).toEqual(["other", "other~net1"]);
    expect(f.state.calls.filter((call) => call.method === "DELETE").map((call) => call.path)).toEqual(["/api/environments/host/networks/net1"]);
  });
});
