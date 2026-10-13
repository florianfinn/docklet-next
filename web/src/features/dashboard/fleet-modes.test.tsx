import { fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import DashboardPage from "./DashboardPage";
import { fleetUnits } from "./fleet-status";
import { DEFAULT_FLEET_VIEW, fleetViewStore, mergeFleetView, orderHosts } from "./fleet-view-store";
import { FleetLine } from "./FleetLine";
import { FleetRing } from "./FleetRing";
import { fanOffset, glowRegion, layoutRing, ringGeometry, strokedArcBox } from "./ring-geometry";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});

afterEach(() => {
  fleetViewStore.reset();
  simulationStore.reset();
  localStorage.clear();
});

/** Stack units from the demo data as the dashboard builds them (system agent already excluded). */
async function demoUnits(hosts = HOSTS.filter((h) => h.status !== "pending")) {
  const adapter = createDemoAdapter();
  const [stacks, containers] = await Promise.all([adapter.listStacks(), adapter.listContainers()]);
  return { units: fleetUnits(hosts, stacks, containers), containers };
}
const inside = (a: { x: number; y: number; width: number; height: number }, b: typeof a) => a.x >= b.x && a.y >= b.y && a.x + a.width <= b.x + b.width && a.y + a.height <= b.y + b.height;

describe("fleet ring fan", () => {
  it("moves only the active segment radially outward, never into the host arc, with the glow region intact", () => {
    for (const size of [160, 220, 320]) {
      const g = ringGeometry(size);
      // Fanned outer edge stays inside the host arc's inner edge.
      expect(g.segR + g.segW / 2 + g.fan < g.hostR - g.hostW / 2).toBe(true);
      const [group] = layoutRing([{ group: "h", items: Array.from({ length: 24 }, (_, i) => i) }]);
      for (const seg of group!.segments) expect(inside(strokedArcBox(size / 2, size / 2, g.segR, seg.start, seg.end, g.segW), glowRegion(size, 3))).toBe(true);
    }
    // 3 o'clock moves right, 6 o'clock down, 9 o'clock left; length equals the fan distance.
    expect([90, 180, 270].map((deg) => fanOffset(deg, 7)).map((o) => [o.x + 0, o.y + 0])).toEqual([
      [7, 0],
      [0, 7],
      [-7, 0],
    ]);
  });

  it("fans the focused segment (keyboard equivalent of hover) and nothing when the lab switch is off", async () => {
    const hosts = HOSTS.filter((h) => h.id === "atlas");
    const { units: own } = await demoUnits(hosts);
    const { unmount } = renderWithApp(<FleetRing hosts={hosts} units={own} label="Ring" center={{ value: "1", caption: "x" }} />);
    const segs = screen.getAllByRole("button");
    fireEvent.focus(segs[2]!);
    const fanned = document.querySelectorAll(".ring-seg-group[data-fanned]");
    expect(fanned.length).toBe(1);
    expect(fanned[0]?.contains(segs[2]!)).toBe(true);
    const g = fanned[0] as SVGGElement;
    expect(g.style.getPropertyValue("--fx") !== "" && g.style.getPropertyValue("--fy") !== "").toBe(true);
    // The full arc shape is kept: the path itself is unchanged, only its group moves.
    expect(segs[2]!.getAttribute("d")?.includes(" A")).toBe(true);
    unmount();

    renderWithApp(<FleetRing hosts={hosts} units={own} label="Ring" center={{ value: "1", caption: "x" }} fan={false} />);
    fireEvent.focus(screen.getAllByRole("button")[2]!);
    expect(document.querySelectorAll(".ring-seg-group[data-fanned]").length).toBe(0);
  });
});

describe("fleet modes", () => {
  it("draws status lines per host with one segment per stack or origin group, far fewer than containers", async () => {
    const { units, containers } = await demoUnits();
    renderWithApp(<FleetLine hosts={HOSTS.filter((h) => h.status !== "pending")} units={units} label="Linien" />);
    const group = screen.getByRole("group", { name: "Linien" });
    const segs = within(group).getAllByRole("button");
    expect(segs.length === units.length && segs.length < containers.length).toBe(true);
    // Every container belongs to exactly one segment.
    expect(units.reduce((n, u) => n + u.containers.length, 0)).toBe(containers.filter((c) => HOSTS.some((h) => h.id === c.hostId && h.status !== "pending")).length);
    const unreachable = segs.filter((s) => s.getAttribute("data-runtime") === "unreachable");
    expect(unreachable.length).toBe(units.filter((u) => u.state === "unreachable").length);
    expect(unreachable.every((s) => s.className.includes("border-dashed"))).toBe(true);
    // One tab stop, arrows move between segments.
    expect(segs.filter((s) => s.tabIndex === 0).length).toBe(1);
    segs[0]!.focus();
    fireEvent.keyDown(segs[0]!, { key: "ArrowRight" });
    expect(document.activeElement).toBe(segs[1]);
  });

  it("shows one ring per host or status lines on the dashboard, in the saved host order", async () => {
    fleetViewStore.set({ ...DEFAULT_FLEET_VIEW, mode: "perHost", hostOrder: ["delta"] });
    const { unmount } = renderWithApp(<DashboardPage />);
    const rings = await screen.findAllByRole("group", { name: /^Stackring (Atlas|Boreal|Cedar|Delta):/ });
    expect(rings.map((r) => r.getAttribute("aria-label")?.split(":")[0])).toEqual(["Stackring Delta", "Stackring Atlas", "Stackring Boreal", "Stackring Cedar"]);
    expect([...document.querySelectorAll("[data-host-card]")].map((c) => c.getAttribute("data-host-card"))).toEqual(["delta", "atlas", "boreal", "cedar"]);
    unmount();

    fleetViewStore.set({ ...DEFAULT_FLEET_VIEW, mode: "line", placement: "right" });
    renderWithApp(<DashboardPage />);
    expect((await screen.findByRole("group", { name: /^Statuslinien aller Hosts/ })) instanceof HTMLElement).toBe(true);
    expect(document.querySelector('[data-slot="host-cards"]')?.getAttribute("data-placement")).toBe("right");
  });

  it("sanitises stored fleet settings", () => {
    expect(mergeFleetView({ mode: "spiral", ringSize: 999, placement: "left", fan: "yes", hostOrder: ["b", 3, "b", "a"] }, DEFAULT_FLEET_VIEW)).toEqual({ mode: "joint", ringSize: 320, placement: DEFAULT_FLEET_VIEW.placement, fan: true, hostOrder: ["b", "a"], hostArea: DEFAULT_FLEET_VIEW.hostArea });
    // Host area size: whole cells within its limits; missing or broken values keep the default.
    expect([mergeFleetView({ hostArea: { w: 30, h: 1.6 } }, DEFAULT_FLEET_VIEW).hostArea, mergeFleetView({ hostArea: { w: "x" } }, DEFAULT_FLEET_VIEW).hostArea, mergeFleetView({ hostArea: 5 }, DEFAULT_FLEET_VIEW).hostArea]).toEqual([
      { w: 9, h: 2 },
      DEFAULT_FLEET_VIEW.hostArea,
      DEFAULT_FLEET_VIEW.hostArea,
    ]);
    const hosts = HOSTS.filter((h) => ["atlas", "boreal", "cedar", "delta"].includes(h.id));
    expect(orderHosts(hosts, ["cedar", "gone"]).map((h) => h.id)).toEqual(["cedar", "atlas", "boreal", "delta"]);
  });
});
