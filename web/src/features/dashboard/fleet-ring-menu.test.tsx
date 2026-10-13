import { act, fireEvent, screen, waitFor, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { simulationStore } from "@/data/demo/simulation";
import { renderWithApp } from "@/test/render";

import { guardUnreachable, placeInfoCard } from "./fleet-segments";
import { fleetUnits } from "./fleet-status";
import { FleetRing } from "./FleetRing";

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
  return { adapter, hosts, units: fleetUnits(hosts, stacks, containers) };
}

const overlaps = (a: { left: number; top: number; right: number; bottom: number }, b: typeof a) => a.left < b.right && a.right > b.left && a.top < b.bottom && a.bottom > b.top;

describe("ring info card placement", () => {
  it("never covers the ring: beside it on the segment's side when it fits, else the other side, else below or above", () => {
    const card = { width: 288, height: 260 };
    const viewport = { width: 1400, height: 900 };
    const cases: [DOMRect, boolean, number, string][] = [
      [new DOMRect(600, 200, 220, 220), true, 0, "left"],
      [new DOMRect(600, 200, 220, 220), false, 0, "right"],
      [new DOMRect(300, 200, 220, 220), true, 280, "right"],
      [new DOMRect(1100, 200, 220, 220), false, 0, "left"],
      [new DOMRect(40, 100, 320, 320), true, 0, "below"],
    ];
    for (const [ring, preferLeft, bound, side] of cases) {
      const vp = side === "below" ? { width: 400, height: 900 } : viewport;
      const p = placeInfoCard(ring, { x: ring.left + 10, y: ring.top + 30 }, card, vp, bound, preferLeft);
      expect(p.side, `${side}`).toBe(side);
      const box = { left: p.left, top: p.top, right: p.left + card.width, bottom: p.top + card.height };
      expect(overlaps(box, ring), `${side} overlaps the ring`).toBe(false);
      expect(p.left >= Math.max(8, side === "left" ? bound + 8 : 8) && p.top >= 8).toBe(true);
    }
  });

  it("shortens the card on a phone rather than covering the ring when neither above nor below holds it", () => {
    const card = { width: 288, height: 300 };
    const phone = { width: 390, height: 640 };
    for (const top of [40, 160, 290]) {
      const ring = new DOMRect(35, top, 320, 320);
      const p = placeInfoCard(ring, { x: 200, y: top + 20 }, card, phone, 0, false);
      const box = { left: p.left, top: p.top, right: p.left + card.width, bottom: p.top + (p.maxHeight ?? card.height) };
      expect([overlaps(box, ring), box.top >= 8, box.bottom <= phone.height - 8, (p.maxHeight ?? 0) >= 120], `ring at ${top}`).toEqual([false, true, true, true]);
    }
    // Room for the whole card: unchanged, no shortening.
    expect(placeInfoCard(new DOMRect(35, 20, 320, 320), { x: 200, y: 40 }, card, { width: 390, height: 844 }, 0, false)).toEqual({ left: 51, top: 352, side: "below" });
  });

  it("disables host commands of disconnected hosts and keeps navigation", () => {
    const noop = () => {};
    const groups = [[{ id: "open", label: "", icon: null, onSelect: noop }], [{ id: "restart", label: "", icon: null, onSelect: noop }, { id: "member:x", label: "", icon: null, onSelect: noop }], [{ id: "hide", label: "", icon: null, onSelect: noop }]];
    const guarded = guardUnreachable(groups, true).flat();
    expect(guarded.map((a) => [a.id, !!a.disabled])).toEqual([
      ["open", false],
      ["restart", true],
      ["member:x", false],
      ["hide", false],
    ]);
    expect(guardUnreachable(groups, false)).toBe(groups);
  });
});

describe("ring hover and actions", () => {
  it("shows a read-only card that takes no pointer events and follows the pointer along the ring without closing", async () => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    const { hosts, units } = await demo();
    renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} />);
    const segs = screen.getAllByRole("button");
    fireEvent.pointerEnter(segs[0]!, { pointerType: "mouse" });
    await act(() => vi.advanceTimersByTimeAsync(200));
    const card = screen.getByRole("tooltip");
    expect([card.className.includes("pointer-events-none"), card.querySelectorAll("a, button, input, [tabindex]").length, segs[0]!.getAttribute("aria-describedby") === card.id]).toEqual([true, 0, true]);
    expect(card.textContent?.includes("Rechtsklick")).toBe(true);
    // Moving to the next segment swaps the content at once; the card stays open.
    fireEvent.pointerEnter(segs[1]!, { pointerType: "mouse" });
    expect(screen.getByRole("tooltip") === card && card.getAttribute("aria-label")?.includes(units[1]!.name)).toBe(true);
    // Leaving the ring closes it after the short delay.
    fireEvent.pointerLeave(segs[1]!.closest("svg")!, { pointerType: "mouse" });
    await act(() => vi.advanceTimersByTimeAsync(300));
    expect(screen.queryByRole("tooltip")).toBe(null);
    vi.useRealTimers();
  });

  it("opens the stack's actions on right click and runs the same command as the row menu", async () => {
    const { adapter, hosts, units } = await demo();
    const run = vi.spyOn(adapter, "runStackCommand");
    renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} />, adapter);
    const index = units.findIndex((u) => u.kind === "stack" && u.state === "running");
    const seg = screen.getAllByRole("button")[index]!;
    fireEvent.contextMenu(seg, { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu", { name: `Aktionen für ${units[index]!.name}` });
    await within(menu).findByRole("menuitem", { name: /Stoppen/ });
    const labels = within(menu).getAllByRole("menuitem").map((m) => m.textContent?.trim());
    expect(["Öffnen", "Stoppen", "Neustart"].every((l) => labels.some((x) => x?.startsWith(l)))).toBe(true);
    // No info card while the menu is open.
    expect(screen.queryByRole("tooltip")).toBe(null);
    fireEvent.click(within(menu).getAllByRole("menuitem").find((m) => m.textContent?.trim().startsWith("Neustart"))!);
    await waitFor(() => expect(run).toHaveBeenCalledWith(units[index]!.stackId, "restart", undefined));
  });

  it("offers the menu by keyboard (Shift+F10) and locks commands for a disconnected host", async () => {
    const { hosts, units } = await demo();
    renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} />);
    const index = units.findIndex((u) => u.state === "unreachable" && !!u.stackId);
    const seg = screen.getAllByRole("button")[index]!;
    act(() => seg.focus());
    fireEvent.keyDown(seg, { key: "F10", shiftKey: true });
    const menu = await screen.findByRole("menu");
    await within(menu).findByRole("menuitem", { name: /Logs/ });
    expect(within(menu).getByText("Host getrennt – Befehle gesperrt") !== null).toBe(true);
    const items = within(menu).getAllByRole("menuitem");
    const disabled = (label: string) => items.find((m) => m.textContent?.trim().startsWith(label))?.hasAttribute("data-disabled");
    expect([disabled("Öffnen"), disabled("Logs"), items.some((m) => m.hasAttribute("data-disabled"))]).toEqual([false, false, true]);
  });

  it("keeps the standard menu slot and leaves nothing behind after Escape", async () => {
    const { hosts, units } = await demo();
    renderWithApp(<FleetRing hosts={hosts} units={units} label="Ring" center={{ value: "", caption: "" }} />);
    const seg = screen.getAllByRole("button")[units.findIndex((u) => u.state === "running")]!;
    fireEvent.contextMenu(seg, { clientX: 50, clientY: 50 });
    const menu = await screen.findByRole("menu");
    // Scroll cues skip "…-content" surfaces; a replaced slot would freeze the exit animation in a browser.
    expect([menu.getAttribute("data-slot"), menu.hasAttribute("data-segment-menu")]).toEqual(["context-menu-content", true]);
    fireEvent.keyDown(menu, { key: "Escape" });
    await waitFor(() => expect(screen.queryByRole("menu")).toBe(null));
    expect(document.querySelectorAll("[data-segment-menu], [data-radix-popper-content-wrapper]").length).toBe(0);
  });
});
