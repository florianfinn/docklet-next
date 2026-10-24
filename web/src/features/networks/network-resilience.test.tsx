// Source: docklet-next/issues/204
// Revision: def05d5f7606c94c26f8fdeb1b16c53c473a0c11
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original partial-read UI and retained-query regression coverage.

import { act, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { afterEach, describe, expect, it } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { useTopology } from "@/data/queries";
import { topologyFixture } from "@/data/http/topology-resilience.test-support";
import type { TopologyNetwork } from "@/domain/model";
import { checkConnect, checkDisconnect, removeBlock } from "@/domain/networks";
import { renderRoutes, renderWithApp } from "@/test/render";
import { ConnectNetworkDialog } from "./ConnectNetworkDialog";
import NetworksPage from "./NetworksPage";

function QueryProbe() {
  const query = useTopology();
  const [completed, setCompleted] = useState(0);
  return <><output data-testid="query-state">{query.fetchStatus}:{query.data?.networks.length}</output><output data-testid="completed">{completed}</output><button onClick={async () => { await query.refetch(); setCompleted((n) => n + 1); }}>Refresh topology</button></>;
}
function page(f: ReturnType<typeof topologyFixture>, host = "second") {
  return renderRoutes([{ path: "/networks", element: <><QueryProbe /><NetworksPage /></> }], [`/networks?host=${host}`], f.adapter);
}
async function settled(count: number) {
  await waitFor(() => expect(screen.getByTestId("query-state").textContent).toBe(`idle:${count}`));
}
const selected = () => screen.queryByRole("radio", { name: "second" })?.getAttribute("aria-checked");
afterEach(() => setAppLocale("de"));

describe.each([{ transport: "Direct", isEdge: false }, { transport: "Edge", isEdge: true }])("$transport resilient Networks UI", ({ isEdge }) => {
  it.each(["en-US", "de"] as const)("keeps the requested partial host, marks missing details and blocks delete in %s", async (locale) => {
    setAppLocale(locale);
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/networks/missing");
    const view = page(f);
    await settled(4);
    expect(selected()).toBe("true");
    expect(view.router.state.location.search).toBe("?host=second");
    const partial = locale === "de" ? "Netzwerkdetails nicht verfügbar" : "Network details unavailable";
    expect(view.container.textContent?.includes(partial)).toBe(true);
    expect(screen.queryByRole("button", { name: locale === "de" ? "unavailable: Verwendung unbekannt" : "unavailable: Usage unknown" }) !== null).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: locale === "de" ? "Aktionen für unavailable" : "Actions for unavailable" }));
    const removeItem = screen.getByRole("menuitem", { name: locale === "de" ? "Entfernen" : "Remove" });
    expect(removeItem.getAttribute("aria-disabled")).toBe("true");
    expect(removeItem.title).toContain(locale === "de" ? "Verwendung kann nicht geprüft" : "usage cannot be verified");
    await userEvent.keyboard("{Escape}");
    const focus = (name: string) => screen.getByRole("button", { name: locale === "de" ? `${name} in der Topologie fokussieren` : `Focus ${name} in the topology` });
    await userEvent.click(focus("healthy"));
    const removeLabel = locale === "de" ? "Netzwerk entfernen" : "Remove network";
    // Prove permissions have resolved before checking the unavailable network.
    await waitFor(() => expect((screen.getByRole("button", { name: removeLabel }) as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(focus("unavailable"));
    expect((screen.getByRole("button", { name: removeLabel }) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByRole("button", { name: removeLabel }).title).toContain(locale === "de" ? "Verwendung kann nicht geprüft" : "usage cannot be verified");
    expect(view.container.querySelector('[aria-label="unavailable"]')?.textContent?.includes(partial)).toBe(true);
  });

  it("keeps an explicitly requested empty host after its initial list fails", async () => {
    setAppLocale("en-US");
    const f = topologyFixture(isEdge);
    f.failures.add("/api/environments/second/networks");
    const view = page(f);
    await settled(2);
    expect(selected()).toBe("true");
    expect(screen.queryByRole("radio", { name: "first" }) !== null).toBe(true);
    expect(screen.queryByRole("heading", { name: "Networks on second" }) !== null).toBe(true);
    expect(view.container.textContent?.includes("second: Some network reads failed")).toBe(true);
    expect(view.router.state.location.search).toBe("?host=second");
  });

  it("keeps an empty online host selectable even without a requested host", async () => {
    setAppLocale("en-US");
    const f = topologyFixture(isEdge);
    f.state.networks = [];
    const view = page(f, "first");
    await settled(0);
    expect(within(screen.getByRole("radiogroup", { name: "Topology host" })).getAllByRole("radio")).toHaveLength(2);
    await userEvent.click(screen.getByRole("radio", { name: "second" }));
    expect(selected()).toBe("true");
    expect(view.router.state.location.search).toBe("?host=second");
  });

  it("uses the existing query snapshot across delayed refetch, list failure and recovery", async () => {
    setAppLocale("en-US");
    const f = topologyFixture(isEdge);
    const view = page(f);
    await settled(4);
    const hold = f.delay("/api/environments/first/networks");
    f.failures.add("/api/environments/second/networks");
    await userEvent.click(screen.getByRole("button", { name: "Refresh topology" }));
    await hold.entered.promise;
    expect(screen.queryByRole("button", { name: "Focus unavailable in the topology" }) !== null).toBe(true);
    expect(selected()).toBe("true");
    await act(async () => { hold.done.release(); });
    await waitFor(() => expect(screen.getByTestId("completed").textContent).toBe("1"));
    await settled(4);
    expect(view.container.textContent?.includes("second: Some network reads failed")).toBe(true);
    expect(screen.queryByRole("button", { name: "unavailable: Usage unknown" }) !== null).toBe(true);
    f.failures.clear();
    await userEvent.click(screen.getByRole("button", { name: "Refresh topology" }));
    await waitFor(() => expect(screen.getByTestId("completed").textContent).toBe("2"));
    expect(view.container.textContent?.includes("Some network reads failed")).toBe(false);
    expect(view.container.textContent?.includes("Network details unavailable")).toBe(false);
    expect(screen.queryByRole("button", { name: "unavailable: Unused, 0 containers" }) !== null).toBe(true);
    expect(selected()).toBe("true");
  });

  it("blocks an already-open connection wizard and omits address hints when detail becomes unavailable", async () => {
    setAppLocale("en-US");
    const f = topologyFixture(isEdge);
    const topology = await f.adapter.getTopology();
    const network = topology.networks.find((n) => n.id === "second~missing")!;
    const host = (await f.adapter.listHosts()).find((h) => h.id === "second")!;
    const containers = await f.adapter.listContainers();
    let loseDetail!: () => void;
    function Wizard() {
      const [unavailable, setUnavailable] = useState(false);
      loseDetail = () => setUnavailable(true);
      const current = { ...topology, networks: topology.networks.map((n) => n.id === network.id ? { ...n, detailUnavailable: unavailable } : n) };
      return <ConnectNetworkDialog request={{ mode: "connect", networkId: network.id, containerId: "second~web" }} onClose={() => {}} host={host} topology={current} containers={containers} />;
    }
    renderWithApp(<Wizard />, f.adapter);
    const dialog = await screen.findByRole("dialog");
    await waitFor(() => expect((within(dialog).getByRole("button", { name: "Review" }) as HTMLButtonElement).disabled).toBe(false));
    await userEvent.click(within(dialog).getByRole("button", { name: "Review" }));
    act(() => loseDetail());
    expect((within(dialog).getByRole("button", { name: "Connect" }) as HTMLButtonElement).disabled).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Back" }));
    expect(dialog.textContent?.includes("usage cannot be verified")).toBe(true);
    expect((within(dialog).getByLabelText("Static address (optional)") as HTMLInputElement).disabled).toBe(true);
    expect((within(dialog).getByLabelText("Static address (optional)") as HTMLInputElement).placeholder).toBe("");
    expect((within(dialog).getByRole("button", { name: "Review" }) as HTMLButtonElement).disabled).toBe(true);
  });
});

it("refuses deletion and address suggestions when network usage is unknown", () => {
  const network: TopologyNetwork = { id: "host~network", hostId: "host", name: "example", driver: "bridge", internal: false, subnet: "192.0.2.0/24", subnetConfigured: true, detailUnavailable: true };
  const topology = { networks: [network], attachments: [], dependencies: [] };
  const container = { id: "host~container", hostId: "host", name: "example", origin: "standalone" as const };
  expect(removeBlock(topology, network)).toBe("detail-unavailable");
  expect(checkConnect(topology, container, network)).toEqual({ block: "detail-unavailable", warnings: [] });
  expect(checkDisconnect(topology, container, network).block).toBe("detail-unavailable");
});
