import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { simulationStore } from "@/data/demo/simulation";
import { COLLATOR } from "@/domain/hosts";
import type { ContainerInfo, Host, MetricsSnapshot } from "@/domain/model";
import { sortStacks } from "@/features/stacks/overview/stack-filters";
import BackupsPage from "@/features/volumes/BackupsPage";
import { STACKS } from "@/data/demo/fixtures/stacks";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";

import { sortContainers } from "./container-filters";
import ContainersPage from "./ContainersPage";

beforeEach(() => {
  // These tests cover the one-table overviews; the shipped default shows host blocks.
  preferencesStore.set((p) => ({ ...p, stacksLayout: "table", containersLayout: "table", volumesLayout: "table", imagesLayout: "table" }));
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  preferencesStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

const rowNames = () => screen.getAllByRole("button", { name: /^Details zu .+ öffnen$/ }).map((b) => b.textContent);
const loadHeader = () => screen.getByRole("button", { name: /^Nach Last \((CPU|RAM)\) sortieren$/ }).closest("th") as HTMLElement;

function snapshotOf(adapter: ReturnType<typeof createDemoAdapter>): MetricsSnapshot {
  let snap: MetricsSnapshot | undefined;
  adapter.subscribeMetrics((s) => (snap = s))();
  return snap!;
}

/** Expected order: measured values in the given direction, unmeasured last, ties by host name and name. */
function expected(containers: ContainerInfo[], hosts: Host[], snap: MetricsSnapshot, metric: "cpu" | "memMb", dir: 1 | -1) {
  const hostName = (c: ContainerInfo) => hosts.find((h) => h.id === c.hostId)?.name ?? c.hostId;
  const value = (c: ContainerInfo) => {
    const online = hosts.find((h) => h.id === c.hostId)?.status === "online";
    const m = snap.containers[c.id];
    return online && c.state === "running" && m && m.memMb > 0 ? m[metric] : null;
  };
  return containers
    .toSorted((a, b) => {
      const va = value(a);
      const vb = value(b);
      if (va === null || vb === null) return (va === null ? 1 : 0) - (vb === null ? 1 : 0) || COLLATOR.compare(hostName(a), hostName(b)) || COLLATOR.compare(a.name, b.name);
      return (va - vb) * dir || COLLATOR.compare(hostName(a), hostName(b)) || COLLATOR.compare(a.name, b.name);
    })
    .map((c) => c.name);
}

describe("load sorting", () => {
  it("sorts containers by live CPU or RAM from the load header, unmeasured last in both directions", async () => {
    const adapter = createDemoAdapter();
    renderWithApp(<ContainersPage />, adapter);
    await screen.findByRole("button", { name: "Details zu duplicati öffnen" });
    const containers = await adapter.listContainers();
    const hosts = await adapter.listHosts();
    const snap = snapshotOf(adapter);
    expect(loadHeader().getAttribute("aria-sort")).toBe("none");

    await userEvent.click(screen.getByRole("button", { name: "Nach Last (CPU) sortieren" }));
    expect(loadHeader().getAttribute("aria-sort")).toBe("descending");
    const visible = containers.filter((c) => rowNames().includes(c.name));
    expect(rowNames()).toEqual(expected(visible, hosts, snap, "cpu", -1));
    // Stopped containers and those on the disconnected host Cedar come last.
    expect(rowNames().slice(-4).toSorted()).toEqual(["git-forgejo-1", "whoami-test", "wiki-db-1", "wiki-wikijs-1"]);

    // The metric choice is a separate control: choosing RAM keeps the direction.
    await userEvent.click(within(screen.getByRole("radiogroup", { name: "Lastwert für Anzeige-Sortierung" })).getByRole("radio", { name: "RAM" }));
    expect(loadHeader().getAttribute("aria-sort")).toBe("descending");
    expect(rowNames()).toEqual(expected(visible, hosts, snap, "memMb", -1));

    await userEvent.click(screen.getByRole("button", { name: "Nach Last (RAM) sortieren" }));
    expect(loadHeader().getAttribute("aria-sort")).toBe("ascending");
    expect(rowNames()).toEqual(expected(visible, hosts, snap, "memMb", 1));
    expect(rowNames().slice(-4).toSorted()).toEqual(["git-forgejo-1", "whoami-test", "wiki-db-1", "wiki-wikijs-1"]);

    // Default order is still reachable from the host header.
    await userEvent.click(screen.getByRole("button", { name: "Nach Host sortieren" }));
    expect(loadHeader().getAttribute("aria-sort")).toBe("none");
  }, 15_000);

  it("orders numerically with unknown values last and deterministic host/name ties", () => {
    const hosts = new Map(HOSTS.map((h) => [h.id, h]));
    const base = { image: { repository: "x", tag: "1" }, origin: "standalone", health: "none", ports: [], networks: [], restartPolicy: "no", memLimitMb: 512, restarts24h: 0, update: { state: "local", current: { repository: "x", tag: "1" } }, labels: {} } as unknown as ContainerInfo;
    const c = (id: string, hostId: string, state: ContainerInfo["state"] = "running") => ({ ...base, id, name: id, hostId, state }) as ContainerInfo;
    const rows = [c("b", "boreal"), c("a", "boreal"), c("z", "atlas"), c("off", "cedar"), c("stopped", "atlas", "stopped"), c("big", "delta")];
    const snapshot = { at: "", simulated: true, hosts: {}, containers: { a: { cpu: 9, memMb: 100 }, b: { cpu: 9, memMb: 100 }, z: { cpu: 9, memMb: 100 }, big: { cpu: 10.5, memMb: 50 }, off: { cpu: 99, memMb: 900 }, stopped: { cpu: 0, memMb: 0 } } } as MetricsSnapshot;
    const order = (dir: "asc" | "desc", metric: "cpu" | "mem") => sortContainers(rows, hosts, { key: "load", dir }, { snapshot, metric }).map((r) => r.id);
    // 10.5 > 9 (numeric, not text); equal values by host name (Atlas before Boreal), then name.
    expect(order("desc", "cpu")).toEqual(["big", "z", "a", "b", "stopped", "off"]);
    expect(order("asc", "cpu")).toEqual(["z", "a", "b", "big", "stopped", "off"]);
    expect(order("desc", "mem")).toEqual(["z", "a", "b", "big", "stopped", "off"]);
    // Stacks sum their services; a stack on the disconnected host has no value.
    const stackHosts = new Map(HOSTS.map((h) => [h.id, h]));
    const stackSnap = { at: "", simulated: true, hosts: {}, containers: { "c-home-homeassistant": { cpu: 1, memMb: 10 }, "c-home-mosquitto": { cpu: 30, memMb: 10 }, "c-media-jellyfin": { cpu: 20, memMb: 10 } } } as MetricsSnapshot;
    const stacks = STACKS.filter((s) => ["home", "media", "wiki"].includes(s.id));
    expect(sortStacks(stacks, stackHosts, { key: "load", dir: "desc" }, { snapshot: stackSnap, metric: "cpu" }).map((s) => s.id)).toEqual(["home", "media", "wiki"]);
  });
});

describe("backups overview sorting", () => {
  it("sorts by clickable headers with aria-sort and a quiet highlight", async () => {
    renderWithApp(<BackupsPage />);
    const names = async () => (await screen.findAllByRole("link", { name: /^[a-z_]+$/ })).map((a) => a.textContent);
    const head = (label: string) => screen.getByRole("button", { name: `Nach ${label} sortieren` }).closest("th") as HTMLElement;
    await screen.findByRole("link", { name: "craft_world" });
    // Default: next run first.
    expect(head("Nächster Lauf").getAttribute("aria-sort")).toBe("ascending");
    await userEvent.click(screen.getByRole("button", { name: "Nach Volume sortieren" }));
    expect([head("Volume").getAttribute("aria-sort"), head("Nächster Lauf").getAttribute("aria-sort")]).toEqual(["ascending", "none"]);
    const sorted = await names();
    expect(sorted).toEqual(sorted.toSorted((a, b) => COLLATOR.compare(a ?? "", b ?? "")));
    expect(screen.getByRole("button", { name: "Nach Volume sortieren" }).hasAttribute("data-active")).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Nach Letzter Lauf sortieren" }));
    expect((await names())[0]).toBe("craft_world");
    await userEvent.click(screen.getByRole("button", { name: "Nach Host sortieren" }));
    const rows = screen.getAllByRole("row").slice(1);
    const hostsInOrder = rows.map((r) => within(r).getAllByText(/^(Atlas|Boreal|Delta)$/)[0]?.textContent);
    expect(hostsInOrder).toEqual(hostsInOrder.toSorted());
  });
});
