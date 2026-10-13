import { fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { TOPOLOGY } from "@/data/demo/fixtures/topology";
import type { ContainerInfo, Host } from "@/domain/model";
import { renderRoutes } from "@/test/render";

import { hostAddress } from "./host-address";
import NetworksPage from "./NetworksPage";
import { BLOCK_GAP, containerHeight } from "./topology-geometry";
import { decorate, layoutHost, type ContainerData, type TopoNode } from "./topology-layout";

let CONTAINERS: ContainerInfo[] = [];
beforeEach(async () => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
  CONTAINERS = await createDemoAdapter().listContainers();
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const host = (id: string) => HOSTS.find((h) => h.id === id) as Host;
const layoutOf = (id: string) => {
  const nets = TOPOLOGY.networks.filter((n) => n.hostId === id).map((n) => n.id);
  return layoutHost({ topology: TOPOLOGY, containers: CONTAINERS, host: host(id), expanded: new Set(nets) });
};
const opts = { query: "", onlyProblems: false, showDependencies: true };
const containerData = (nodes: TopoNode[], id: string) => nodes.find((n) => n.id === id)?.data as ContainerData;

describe("host address", () => {
  it("strips scheme, path and agent port from direct hosts; edge hosts have no known address", () => {
    const direct = (address: string) => hostAddress({ ...host("atlas"), address });
    expect([direct("atlas.example.com:3552"), direct("http://192.0.2.40:3552/agent"), direct("[2001:db8::5]:3552"), direct("2001:db8::5"), direct("192.0.2.41")]).toEqual([
      "atlas.example.com",
      "192.0.2.40",
      "2001:db8::5",
      "2001:db8::5",
      "192.0.2.41",
    ]);
    expect([hostAddress(host("cedar")), hostAddress(undefined)]).toEqual([null, null]);
  });
});

describe("focus addresses", () => {
  it("puts the address on the container block, never on the edge, for a focused network", () => {
    const { nodes, edges } = decorate(layoutOf("atlas"), { ...opts, focus: "n-atlas-paperless" });
    expect(containerData(nodes, "c-paperless-webserver").addresses).toEqual([{ networkId: "n-atlas-paperless", network: "paperless_default", kind: "ip", address: "198.51.100.162" }]);
    expect(containerData(nodes, "c-media-jellyfin").addresses).toEqual([]);
    expect(edges.every((e) => e.label === undefined)).toBe(true);
  });

  it("lists every network of a focused container, with the host address in host mode", () => {
    const boreal = decorate(layoutOf("boreal"), { ...opts, focus: "c-cloud-nextcloud" });
    expect(containerData(boreal.nodes, "c-cloud-nextcloud").addresses.map((a) => `${a.network} ${a.address}`)).toEqual(["cloud_web 198.51.100.2", "cloud_internal 198.51.100.18"]);
    const atlas = decorate(layoutOf("atlas"), { ...opts, focus: "c-home-homeassistant" });
    expect(containerData(atlas.nodes, "c-home-homeassistant").addresses).toEqual([{ networkId: "n-atlas-host", network: "host", kind: "host", address: "atlas.example.com" }]);
  });

  it("reserves one line per network address so focus never overlaps the next block", () => {
    // Postgres joins a second network; the block below it in its column reserves a second address line.
    const topology = { ...TOPOLOGY, attachments: [...TOPOLOGY.attachments, { containerId: "c-cloud-postgres", networkId: "n-boreal-web", ip: "198.51.100.3" }] };
    const layout = layoutHost({ topology, containers: CONTAINERS, host: host("boreal"), expanded: new Set(TOPOLOGY.networks.map((n) => n.id)) });
    const links = (id: string) => topology.attachments.filter((a) => a.containerId === id).length;
    const columns = new Map<number, TopoNode[]>();
    for (const n of layout.nodes) if (n.type === "container") columns.set(n.position.x, [...(columns.get(n.position.x) ?? []), n]);
    const gaps: [number, number][] = [];
    for (const col of columns.values()) {
      const sorted = col.toSorted((a, b) => a.position.y - b.position.y);
      for (let i = 1; i < sorted.length; i++) if (layout.homeOf.get(sorted[i]!.id) === layout.homeOf.get(sorted[i - 1]!.id)) gaps.push([sorted[i]!.position.y - sorted[i - 1]!.position.y, containerHeight(links(sorted[i - 1]!.id)) + BLOCK_GAP]);
    }
    expect(gaps.some(([, want]) => want > containerHeight(1) + BLOCK_GAP)).toBe(true);
    expect(gaps.every(([got, want]) => got === want)).toBe(true);
  });

  it("leaves unfocused container nodes untouched, so memoised nodes skip repaints", () => {
    const layout = layoutOf("atlas");
    const unfocused = decorate(layout, { ...opts, focus: null });
    expect(unfocused.nodes.filter((n) => n.type === "container").every((n) => layout.nodes.includes(n))).toBe(true);
  });
});

describe("focus addresses on the page", () => {
  it("shows the member address on the block and the host address in the details", async () => {
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks?node=n-atlas-paperless"]);
    const block = await waitFor(() => {
      const el = document.querySelector<HTMLElement>('.react-flow__node[data-id="c-paperless-webserver"] [data-slot="focus-addresses"]');
      if (!el) throw new Error("addresses missing");
      return el;
    });
    expect(block.textContent).toBe("paperless_default198.51.100.162");
    expect(document.querySelector(".react-flow__edge-text")).toBe(null);

    fireEvent.click(document.querySelector('.react-flow__node[data-id="c-home-homeassistant"]') as HTMLElement);
    const heading = await screen.findByRole("heading", { level: 3, name: /homeassistant/ });
    const panel = heading.closest<HTMLElement>("[aria-label]") as HTMLElement;
    await waitFor(() => expect(within(panel).getByText("atlas.example.com") !== null).toBe(true));
    expect(within(panel).queryByText("keine Adresse")).toBe(null);
  });
});
