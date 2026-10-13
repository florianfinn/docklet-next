import { act, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import { fleetBreakdown, fleetUnits } from "./fleet-status";
import { FleetRing } from "./FleetRing";
import { cellAt, hitBand, hitCells, layoutRing, polar, ringGeometry, segmentFan } from "./ring-geometry";

beforeEach(() => {
  simulationStore.set((s) => ({ ...s, latency: "instant", live: false }));
});
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

async function demo(hosts = HOSTS.filter((h) => h.status !== "pending")) {
  const adapter = createDemoAdapter();
  const [stacks, containers] = await Promise.all([adapter.listStacks(), adapter.listContainers()]);
  return { hosts, units: fleetUnits(hosts, stacks, containers) };
}

describe("ring units", () => {
  it("draws one segment per stack: the disconnected host's two-container stack is a single solid block", async () => {
    const { hosts, units } = await demo();
    const cedar = units.filter((u) => u.hostId === "cedar");
    expect(cedar.map((u) => [u.kind, u.name, u.containers.length, u.state])).toEqual([["stack", "Wiki", 2, "unreachable"]]);
    // 9 stacks plus 3 origin groups of containers outside Compose; 8 confirmed running.
    const b = fleetBreakdown(hosts, units);
    expect([units.filter((u) => u.kind === "stack").length, units.filter((u) => u.kind === "group").length, b.units, b.running + b.starting, b.unreachable, b.problem, b.stopped, b.partial]).toEqual([9, 3, 12, 8, 1, 1, 1, 1]);

    const { container } = renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "8/12", caption: "" }} size={320} />);
    const segs = [...container.querySelectorAll<SVGPathElement>("path[data-runtime]")];
    expect([segs.length, container.querySelectorAll("path[data-hit]").length, segs.filter((p) => p.dataset.runtime === "unreachable").length]).toEqual([12, 12, 1]);
    expect(segs.some((p) => p.hasAttribute("stroke-dasharray"))).toBe(false);
    // The host arc keeps its dashed red context.
    expect(container.querySelector('path[data-host-arc="offline"]')?.hasAttribute("stroke-dasharray")).toBe(true);
  });
});

describe("ring hit cells", () => {
  const groups = layoutRing([
    { group: "a", items: [0, 1, 2, 3, 4] },
    { group: "b", items: [5, 6, 7] },
    { group: "c", items: [8] },
  ]);
  const flat = groups.flatMap((g) => g.segments);

  it("splits neighbours in the middle, pads host group gaps without closing them", () => {
    const pad = 2;
    const cells = hitCells(flat, pad);
    for (const [i, seg] of flat.entries()) {
      const cell = cells[i]!;
      expect(cell.start <= seg.start && cell.end >= seg.end, `${i} covers its segment`).toBe(true);
    }
    // Inside a host group the cells touch exactly; across a group gap a free middle stays.
    const touching = (i: number) => Math.abs(cells[i]!.end - cells[i + 1]!.start) < 1e-9;
    expect([0, 1, 2, 3, 5, 6].every(touching)).toBe(true);
    expect([4, 7].map((i) => cells[i + 1]!.start - cells[i]!.end > 0)).toEqual([true, true]);
    expect([4, 7].every((i) => cells[i]!.end - flat[i]!.end <= pad + 1e-9)).toBe(true);
    // Last cell wraps past 360° and ends before the first one starts.
    expect(cells.at(-1)!.end - 360 < cells[0]!.start).toBe(true);
  });

  it("finds the segment at its resting and fanned position, the nearest neighbour at a boundary and nothing in gaps or the centre", () => {
    // Mixed host groups, two half rings (one host with two stacks) and one single-stack host.
    const layouts = [flat, layoutRing([{ group: "d", items: [0, 1] }])[0]!.segments, layoutRing([{ group: "c", items: [0] }])[0]!.segments];
    for (const size of [160, 220, 320])
      for (const segs of layouts) {
        const g = ringGeometry(size);
        const c = size / 2;
        const band = hitBand(g, true);
        const cells = hitCells(segs, (g.hitPad / g.segR) * (180 / Math.PI));
        const at = (r: number, deg: number, shift = { x: 0, y: 0 }) => {
          const p = polar(c, c, r, deg);
          return cellAt(cells, band, c, c, p.x + shift.x, p.y + shift.y);
        };
        for (const seg of segs) {
          const mid = (seg.start + seg.end) / 2;
          const off = segmentFan(seg.start, seg.end, g.fan);
          // Every corner and the middle of the drawn band, at rest and fanned, lies in its own cell.
          for (const deg of [seg.start + 0.01, mid, seg.end - 0.01])
            for (const r of [g.segR - g.segW / 2, g.segR, g.segR + g.segW / 2]) {
              expect(at(r, deg), `${size}/${segs.length}/${seg.index} rest`).toBe(seg.index);
              expect(at(r, deg, off), `${size}/${segs.length}/${seg.index} fanned`).toBe(seg.index);
            }
          // Slightly beyond the band on both sides still counts.
          expect([at(g.segR - g.segW / 2 - g.hitPad / 2, mid), at(g.segR + g.segW / 2 + g.fan + g.hitPad / 2, mid)]).toEqual([seg.index, seg.index]);
        }
        expect(cellAt(cells, band, c, c, c, c)).toBe(undefined);
      }
    const g = ringGeometry(220);
    const band = hitBand(g, true);
    const cells = hitCells(flat, (g.hitPad / g.segR) * (180 / Math.PI));
    const at = (r: number, deg: number) => cellAt(cells, band, 110, 110, polar(110, 110, r, deg).x, polar(110, 110, r, deg).y);
    // Boundary between neighbours 1 and 2: each side goes to the nearer one.
    const border = (flat[1]!.end + flat[2]!.start) / 2;
    expect([at(g.segR, border - 0.1), at(g.segR, border + 0.1)]).toEqual([1, 2]);
    // Middle of the gap between host groups, inside the ring and outside it stay free.
    expect([at(g.segR, (flat[4]!.end + flat[5]!.start) / 2), at(band.r0 - 1, 30), at(band.r1 + 1, 30)]).toEqual([undefined, undefined, undefined]);
  });

  it("does not fan a segment wider than half the ring, so no part of it moves inward", () => {
    expect([segmentFan(0.9, 359.1, 7), segmentFan(0.9, 179.1, 7).x > 6.9]).toEqual([{ x: 0, y: 0 }, true]);
  });

  it("covers the whole ring for a host with a single stack", () => {
    const [only] = layoutRing([{ group: "cedar", items: ["wiki"] }]);
    const cells = hitCells(only!.segments, 2);
    expect(cells[0]!.end - cells[0]!.start).toBeCloseTo(360, 6);
    const g = ringGeometry(220);
    expect([0, 0.2, 90, 359.8].map((deg) => cellAt(cells, hitBand(g, true), 110, 110, polar(110, 110, g.segR, deg).x, polar(110, 110, g.segR, deg).y))).toEqual([0, 0, 0, 0]);
  });
});

describe("ring hit areas in the DOM", () => {
  it("keeps the hit cell static while the segment fans and opens the segment menu from both", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { hosts, units } = await demo();
    const { container } = renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} size={320} />);
    const index = units.findIndex((u) => u.state === "unreachable");
    const hit = container.querySelector<SVGPathElement>(`path[data-hit="${index}"]`)!;
    const seg = screen.getAllByRole("button")[index]!;
    // Static hit cell next to the moving group, both inside one menu trigger.
    const root = hit.parentElement!;
    expect([root.hasAttribute("data-segment-root"), root.contains(seg), !!hit.closest(".ring-seg-group"), hit.getAttribute("pointer-events"), hit.getAttribute("aria-hidden")]).toEqual([true, true, false, "all", "true"]);
    const g = ringGeometry(320);
    const band = hitBand(g, true);
    expect([hit.getAttribute("d")!.includes(`A${band.r1} ${band.r1}`), hit.getAttribute("d")!.includes(`A${band.r0} ${band.r0}`)]).toEqual([true, true]);

    // Hover on the inner part of the cell fans the segment; the card takes no pointer events.
    fireEvent.pointerEnter(hit, { pointerType: "mouse" });
    await act(() => vi.advanceTimersByTimeAsync(200));
    expect([!!seg.closest(".ring-seg-group[data-fanned]"), screen.getByRole("tooltip").className.includes("pointer-events-none")]).toEqual([true, true]);

    // Right click on the old (inner) place, via the static cell.
    fireEvent.contextMenu(hit, { clientX: 40, clientY: 160 });
    const menu = await screen.findByRole("menu", { name: `Aktionen für ${units[index]!.name}` });
    await within(menu).findByRole("menuitem", { name: /Logs/ });
    expect(within(menu).getAllByRole("menuitem").some((m) => m.hasAttribute("data-disabled"))).toBe(true);
    expect(screen.queryByRole("tooltip")).toBe(null);
    fireEvent.keyDown(menu, { key: "Escape" });
    await act(() => vi.advanceTimersByTimeAsync(50));
    vi.useRealTimers();

    // Right click on the drawn (fanned) segment opens the same menu.
    fireEvent.contextMenu(seg, { clientX: 30, clientY: 160 });
    expect((await screen.findByRole("menu", { name: `Aktionen für ${units[index]!.name}` })) instanceof HTMLElement).toBe(true);
  });

  it("opens the menu with the context-menu key on the focused segment and gives focus back", async () => {
    const { hosts, units } = await demo();
    renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} />);
    const seg = screen.getAllByRole("button")[1]!;
    act(() => seg.focus());
    fireEvent.keyDown(seg, { key: "ContextMenu" });
    const menu = await screen.findByRole("menu", { name: `Aktionen für ${units[1]!.name}` });
    fireEvent.keyDown(menu, { key: "Escape" });
    await vi.waitFor(() => expect(document.activeElement === seg).toBe(true));
  });

  it("gives every segment in a per-host ring its own cell, including the single-stack host", async () => {
    const { units } = await demo();
    for (const host of HOSTS.filter((h) => h.status !== "pending")) {
      const own = units.filter((u) => u.hostId === host.id);
      const { container, unmount } = renderWithApp(<FleetRing hosts={[host]} units={own} label={host.name} center={{ value: "", caption: "" }} size={211} />);
      expect([container.querySelectorAll("path[data-hit]").length, container.querySelectorAll("path[data-runtime]").length]).toEqual([own.length, own.length]);
      unmount();
    }
  });
});
