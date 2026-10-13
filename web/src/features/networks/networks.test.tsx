import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { errorCode } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import type { NetworkDraft } from "@/domain/networks";
import { renderRoutes } from "@/test/render";

import NetworksPage from "./NetworksPage";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const node = (id: string) => {
  const el = document.querySelector<HTMLElement>(`.react-flow__node[data-id="${id}"]`);
  if (!el) throw new Error(`node ${id} not rendered`);
  return el;
};
const details = () => screen.getByRole("heading", { level: 3 }).textContent;
const draft = (p: Partial<NetworkDraft>): NetworkDraft => ({ hostId: "boreal", name: "lab_net", driver: "bridge", subnet: "", gateway: "", ipRange: "", parent: "", internal: false, ...p });

describe("topology focus", () => {
  it("moves focus between nodes at any time and clears it with Escape, the pane and the clear button", async () => {
    const { router } = renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks"]);
    await screen.findByRole("radiogroup", { name: "Host der Topologie" });
    await waitFor(() => node("n-atlas-media"));

    fireEvent.click(node("n-atlas-media"));
    await waitFor(() => expect(details()).toBe("media_default"));
    expect(router.state.location.search).toBe("?node=n-atlas-media");
    // A second node replaces the focus instead of being ignored.
    fireEvent.click(node("c-paperless-db"));
    await waitFor(() => expect(details()).toBe("paperless-db-1"));
    expect(node("c-paperless-db").classList.contains("selected")).toBe(true);
    expect(node("n-atlas-media").classList.contains("selected")).toBe(false);

    fireEvent.keyDown(node("c-paperless-db"), { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("heading", { level: 3 }) === null).toBe(true));
    expect(router.state.location.search).toBe("");

    // Keyboard: Enter on a focused node takes the focus.
    node("n-atlas-br0").focus();
    fireEvent.keyDown(node("n-atlas-br0"), { key: "Enter" });
    await waitFor(() => expect(details()).toBe("br0"));
    fireEvent.click(document.querySelector(".react-flow__pane")!);
    await waitFor(() => expect(router.state.location.search).toBe(""));

    await userEvent.click(screen.getByRole("button", { name: "br0 in der Topologie fokussieren" }));
    await waitFor(() => expect(details()).toBe("br0"));
    await userEvent.click(screen.getAllByRole("button", { name: "Fokus aufheben" })[0]!);
    await waitFor(() => expect(router.state.location.search).toBe(""));
  });

  it("shows configured dependencies instead of observed traffic", async () => {
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?node=c-paperless-webserver"]);
    const card = await screen.findByLabelText("paperless-webserver-1");
    // broker waits for webserver; webserver itself waits for nothing and db is not involved.
    expect(within(card).getByRole("button", { name: "broker" }) !== null && within(card).queryByRole("button", { name: "db" }) === null).toBe(true);
    expect(screen.queryByText(/Beobachtet/) === null && screen.queryByText(/Bewegung/) === null).toBe(true);
  });
});

describe("network wizards", () => {
  it("connects a container through choice, review and confirmation and refreshes the view", async () => {
    const adapter = createDemoAdapter();
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=boreal&node=c-uptime-kuma"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Mit Netzwerk verbinden" }));
    const dialog = await screen.findByRole("dialog");
    expect((within(dialog).getByRole("button", { name: "Prüfen" }) as HTMLButtonElement).disabled).toBe(true);

    within(dialog).getByRole("combobox", { name: "Netzwerk" }).focus();
    await userEvent.keyboard("{Enter}");
    // Docker's host network stays visible but cannot be picked.
    expect((await screen.findByRole("option", { name: /^host/ })).getAttribute("aria-disabled")).toBe("true");
    await userEvent.click(screen.getByRole("option", { name: /^shared_proxy/ }));
    await userEvent.type(within(dialog).getByLabelText("Feste Adresse (optional)"), "198.51.100.97");
    expect(within(dialog).getByText("Das ist das Gateway") !== null).toBe(true);
    await userEvent.clear(within(dialog).getByLabelText("Feste Adresse (optional)"));
    await userEvent.type(within(dialog).getByLabelText("Feste Adresse (optional)"), "198.51.100.110");
    await userEvent.click(within(dialog).getByRole("button", { name: "Prüfen" }));
    expect(within(dialog).getByText("198.51.100.110") !== null).toBe(true);
    await userEvent.click(within(dialog).getByRole("button", { name: "Verbinden" }));

    await waitFor(() => expect(screen.queryByRole("dialog") === null).toBe(true));
    const topology = await adapter.getTopology();
    expect(topology.attachments.some((a) => a.containerId === "c-uptime-kuma" && a.networkId === "n-boreal-shared" && a.ip === "198.51.100.110" && a.staticIp)).toBe(true);
    await waitFor(() => expect(screen.getAllByText("198.51.100.110").length > 0).toBe(true));
    expect((await adapter.listContainers()).find((c) => c.id === "c-uptime-kuma")?.networks).toEqual(["bridge", "shared_proxy"]);
  });

  it("creates a bridge through the three steps and focuses it", async () => {
    const adapter = createDemoAdapter();
    const { router } = renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=boreal"], adapter);
    await userEvent.click(await screen.findByRole("button", { name: "Netzwerk anlegen" }));
    const dialog = await screen.findByRole("dialog");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    await userEvent.type(within(dialog).getByLabelText("Name"), "bridge");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    expect(within(dialog).getByText("Reservierter Name von Docker") !== null).toBe(true);
    await userEvent.clear(within(dialog).getByLabelText("Name"));
    await userEvent.type(within(dialog).getByLabelText("Name"), "lab_net");
    await userEvent.type(within(dialog).getByLabelText("IPv4-Subnetz (CIDR)"), "198.51.100.208/28");
    await userEvent.click(within(dialog).getByRole("button", { name: "Weiter" }));
    await userEvent.click(within(dialog).getByRole("button", { name: "Anlegen" }));
    await waitFor(() => expect(router.state.location.search.includes("node=n-boreal-")).toBe(true));
    const created = (await adapter.getTopology()).networks.find((n) => n.name === "lab_net");
    expect([created?.subnet, created?.subnetConfigured, created?.hostId]).toEqual(["198.51.100.208/28", true, "boreal"]);
    await waitFor(() => expect(details()).toBe("lab_net"));
  });
});

describe("demo network contract", () => {
  it("repeats every check on the adapter side", async () => {
    const adapter = createDemoAdapter();
    const code = (p: Promise<unknown>) => p.then(() => "ok").catch((e: unknown) => errorCode(e));
    expect(await code(adapter.createNetwork(draft({ driver: "macvlan" })))).toBe("invalid-network");
    expect(await code(adapter.createNetwork(draft({ name: "cloud_web" })))).toBe("exists");
    expect(await code(adapter.createNetwork(draft({ hostId: "cedar" })))).toBe("host-offline");
    expect(await code(adapter.connectNetwork({ networkId: "n-boreal-shared", containerId: "c-duplicati" }))).toBe("host-mismatch");
    expect(await code(adapter.connectNetwork({ networkId: "n-atlas-media", containerId: "c-home-homeassistant" }))).toBe("network-mode");
    expect(await code(adapter.connectNetwork({ networkId: "n-boreal-web", containerId: "c-uptime-kuma", ip: "198.51.100.9" }))).toBe("invalid-ip");
    expect(await code(adapter.removeNetwork("n-atlas-bridge"))).toBe("builtin-network");
    expect(await code(adapter.removeNetwork("n-atlas-media"))).toBe("compose-managed");
    expect(await code(adapter.removeNetwork("n-atlas-br0"))).toBe("in-use");
    // A disconnected host is never addressed, whatever the network.
    expect(await code(adapter.removeNetwork("n-cedar-bridge"))).toBe("host-offline");
  });

  it("creates with an automatic block, connects from the pool and keeps a container without networks isolated", async () => {
    const adapter = createDemoAdapter();
    const net = await adapter.createNetwork(draft({ name: "auto_net" }));
    expect([net.subnet, net.gateway, net.subnetConfigured]).toEqual(["203.0.113.96/28", "203.0.113.97", false]);
    const a = await adapter.connectNetwork({ networkId: net.id, containerId: "c-whoami-test" });
    expect(a.ip).toBe("203.0.113.98");
    await adapter.disconnectNetwork({ networkId: net.id, containerId: "c-whoami-test" });
    await adapter.disconnectNetwork({ networkId: "n-boreal-bridge", containerId: "c-whoami-test" });
    expect((await adapter.listContainers()).find((c) => c.id === "c-whoami-test")?.networks).toEqual([]);
    await adapter.removeNetwork(net.id);
    expect((await adapter.getTopology()).networks.some((n) => n.id === net.id)).toBe(false);
    expect((await adapter.getTopology()).dependencies.filter((d) => d.from.startsWith("c-paperless-"))).toEqual([{ from: "c-paperless-broker", to: "c-paperless-webserver" }]);
  });
});

describe("network list beside the graph", () => {
  it("filters by the page search (also through member containers) and a group, sorts, resets and stays in step with the focus", async () => {
    const { router } = renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=atlas"]);
    const list = await screen.findByRole("region", { name: "Netzwerke auf Atlas" });
    const names = () => within(list).queryAllByRole("button", { name: /in der Topologie fokussieren$/ }).map((b) => b.getAttribute("aria-label")?.replace(" in der Topologie fokussieren", ""));
    expect(names()).toEqual(["br0", "home_default", "media_default", "paperless_default", "bridge", "host"]);
    // No second search field: the table follows the topology search, including container names.
    await userEvent.type(screen.getByRole("searchbox", { name: "Topologie durchsuchen" }), "broker");
    expect([names(), within(list).getByText("1 von 6") !== null]).toEqual([["paperless_default"], true]);
    await userEvent.clear(screen.getByRole("searchbox", { name: "Topologie durchsuchen" }));
    await userEvent.click(within(list).getByRole("radio", { name: "Docker-Standard" }));
    expect(names()).toEqual(["bridge", "host"]);
    await userEvent.type(screen.getByRole("searchbox", { name: "Topologie durchsuchen" }), "nichts-da");
    expect(within(list).getByText("Kein Netzwerk passt zu Suche und Auswahl.") !== null).toBe(true);
    await userEvent.click(within(list).getAllByRole("button", { name: "Filter zurücksetzen" })[0]!);
    expect([names().length, (screen.getByRole("searchbox", { name: "Topologie durchsuchen" }) as HTMLInputElement).value]).toEqual([6, ""]);
    // Sorting by member count (most first) via the header, with aria-sort.
    await userEvent.click(within(list).getByRole("button", { name: "Nach Container sortieren" }));
    expect([names()[0], within(list).getByRole("columnheader", { name: /Container/ }).getAttribute("aria-sort")]).toEqual(["paperless_default", "descending"]);
    // Selecting a row focuses the graph; a focused container marks its networks.
    await userEvent.click(within(list).getByRole("button", { name: "media_default in der Topologie fokussieren" }));
    expect(router.state.location.search).toContain("node=n-atlas-media");
    expect(within(list).getByRole("button", { name: "media_default in der Topologie fokussieren" }).getAttribute("aria-pressed")).toBe("true");
    await act(() => router.navigate("/networks?host=atlas&node=c-krusader"));
    await waitFor(() => expect(list.querySelector('[data-related="true"]')?.textContent?.startsWith("br0")).toBe(true));
  });

  it("names the active host in the connect, disconnect and create dialogs", async () => {
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?host=delta&node=c-dns-adguard"]);
    await userEvent.click(await screen.findByRole("button", { name: "Mit Netzwerk verbinden" }));
    let scope = (await screen.findByRole("dialog")).querySelector<HTMLElement>('[data-slot="host-scope"]');
    expect([scope?.dataset.host, scope?.textContent?.includes("Delta"), scope?.textContent?.includes("Nur Container und Netzwerke dieses Hosts.")]).toEqual(["delta", true, true]);
    await userEvent.click(within(screen.getByRole("dialog")).getByRole("radio", { name: "Trennen" }));
    scope = screen.getByRole("dialog").querySelector<HTMLElement>('[data-slot="host-scope"]');
    expect(scope?.textContent?.includes("Nur Netzwerke, mit denen der Container auf diesem Host verbunden ist.")).toBe(true);
  });
});
