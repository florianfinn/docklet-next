import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import NetworksPage from "./NetworksPage";
import { benchTopology } from "./topology-bench";
import { decorate, layoutHost } from "./topology-layout";

const sources = import.meta.glob<string>(["/src/features/networks/*.tsx"], { query: "?raw", import: "default", eager: true });

const bench = benchTopology({ host: HOSTS[0]!, container: STANDALONE[0]! }, 3, 600);
const host = bench.hosts[0]!;
const median = (xs: number[]) => xs.toSorted((a, b) => a - b)[Math.floor(xs.length / 2)] ?? 0;

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
  performance.clearMeasures("topology-layout");
});

describe("topology performance", () => {
  it("lays out one host at a time and aggregates networks until they are expanded", () => {
    const collapsed = layoutHost({ topology: bench.topology, containers: bench.containers, host, expanded: new Set() });
    // 60 network nodes plus one lane heading; no containers while collapsed.
    expect([collapsed.stats.networks, collapsed.stats.containers, collapsed.nodes.length, collapsed.memberships.length]).toEqual([60, 600, 61, 0]);
    const all = new Set(bench.topology.networks.filter((n) => n.hostId === host.id).map((n) => n.id));
    const expanded = layoutHost({ topology: bench.topology, containers: bench.containers, host, expanded: all });
    // 60 network nodes + 1 lane + 600 container nodes of this host only; the other 1 200 containers are not laid out.
    expect([expanded.nodes.length, expanded.nodes.every((n) => n.type !== "container" || n.data.container.hostId === host.id)]).toEqual([661, true]);
  });

  it("stays within a frame budget for 600 containers and only decorates on filter changes", () => {
    const all = new Set(bench.topology.networks.filter((n) => n.hostId === host.id).map((n) => n.id));
    const times: number[] = [];
    let layout = layoutHost({ topology: bench.topology, containers: bench.containers, host, expanded: all });
    for (let i = 0; i < 7; i++) {
      const t0 = performance.now();
      layout = layoutHost({ topology: bench.topology, containers: bench.containers, host, expanded: all });
      times.push(performance.now() - t0);
    }
    // Generous bound for slow CI machines; measured values are in the README.
    expect(median(times) < 120).toBe(true);
    const plain = decorate(layout, { query: "", onlyProblems: false, focus: null, showDependencies: true });
    const filtered = decorate(layout, { query: "svc-1", onlyProblems: false, focus: null, showDependencies: true });
    // Filters dim nodes but never move them; untouched nodes keep their identity for React Flow.
    expect(filtered.nodes.every((n, i) => n.position === plain.nodes[i]?.position)).toBe(true);
    expect(plain.nodes.filter((n, i) => n === layout.nodes[i]).length).toBe(layout.nodes.length);
    expect(plain.edges.some((e) => e.animated)).toBe(false);
    expect(plain.edges.filter((e) => e.id.startsWith("d-")).length).toBe(layout.dependencies.length);
  });

  it("keeps live metrics out of the graph nodes and draws no traffic", () => {
    expect(sources["/src/features/networks/TopologyNodes.tsx"]?.includes("useMetrics")).toBe(false);
    const page = sources["/src/features/networks/NetworksPage.tsx"] ?? "";
    expect(page.includes("onlyRenderVisibleElements") && !page.includes("MiniMap") && !page.includes("animated")).toBe(true);
  });

  it("computes the layout once per host and data change, not per selection or search", async () => {
    renderRoutes([{ path: "/networks", element: <NetworksPage /> }], ["/networks"]);
    await screen.findByRole("radiogroup", { name: "Host der Topologie" });
    const count = () => performance.getEntriesByName("topology-layout").length;
    const after = count();
    expect(after >= 1).toBe(true);
    await userEvent.type(screen.getByRole("searchbox", { name: "Topologie durchsuchen" }), "redis");
    await userEvent.click(screen.getByRole("button", { name: "media_default in der Topologie fokussieren" }));
    expect(count()).toBe(after);
    await userEvent.click(screen.getByRole("radio", { name: /Boreal/ }));
    expect(count()).toBe(after + 1);
    expect(screen.getByText("Netzwerke auf Boreal") !== null).toBe(true);
  });
});
