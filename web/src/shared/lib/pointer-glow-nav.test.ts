import { afterEach, describe, expect, it, vi } from "vitest";

import { glowCandidates, glowStart, startPointerGlow } from "./pointer-glow";

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

/**
 * Sidebar with three 32 px rows 4 px apart (as in the free sidebar) plus a
 * card on the right; every element gets a fixed rect and elementFromPoint
 * answers from those rects, innermost first.
 */
function layout() {
  document.body.innerHTML = `
    <ul id="menu">
      <li data-slot="sidebar-menu-item"><a data-slot="sidebar-menu-button" id="hosts"><svg id="hosts-icon"></svg><span>Hosts</span></a><span data-slot="sidebar-menu-badge" id="badge">4</span></li>
      <li data-slot="sidebar-menu-item"><a data-slot="sidebar-menu-button" id="backups"><span>Backups</span></a></li>
      <li data-slot="sidebar-menu-item"><a data-slot="sidebar-menu-button" id="notify"><span id="notify-label">Benachrichtigungen</span></a></li>
    </ul>
    <nav><a class="nav-row" id="s-general">Allgemein</a><a class="nav-row" id="s-look">Darstellung</a></nav>
    <div data-slot="card" id="card"><p id="card-text">x</p></div>`;
  const $ = (id: string) => document.getElementById(id)!;
  const rects: [HTMLElement, DOMRect][] = [
    [$("hosts"), new DOMRect(8, 0, 240, 32)],
    [$("backups"), new DOMRect(8, 36, 240, 32)],
    [$("notify"), new DOMRect(8, 72, 240, 32)],
    [$("s-general"), new DOMRect(8, 200, 240, 36)],
    [$("s-look"), new DOMRect(8, 240, 240, 36)],
    [$("card"), new DOMRect(400, 0, 600, 400)],
  ];
  for (const [el, rect] of rects) vi.spyOn(el, "getBoundingClientRect").mockReturnValue(rect);
  const hit = (x: number, y: number) => rects.find(([, r]) => x >= r.left && x <= r.right && y >= r.top && y <= r.bottom)?.[0] ?? document.body;
  Object.defineProperty(document, "elementFromPoint", { configurable: true, value: hit });
  const frames: FrameRequestCallback[] = [];
  vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => frames.push(cb));
  vi.spyOn(window, "getComputedStyle").mockReturnValue({ borderRadius: "8px" } as CSSStyleDeclaration);
  const stop = startPointerGlow();
  const overlay = document.querySelector<HTMLElement>('[data-slot="pointer-glow"]')!;
  /** Moves the mouse over `target` (default: whatever lies under the point) and paints one frame. */
  const move = (x: number, y: number, target: Element = hit(x, y)) => {
    target.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, clientX: x, clientY: y, pointerType: "mouse" }));
    frames.splice(0).forEach((cb) => cb(0));
    const lit = Number(overlay.style.opacity) > 0;
    const top = /translate\([\d.]+px, ([\d.]+)px\)/.exec(overlay.style.transform)?.[1];
    return { lit, top: lit ? Number(top) : null };
  };
  return { $, move, stop };
}

describe("edge light in the navigation", () => {
  it("never moves to the row above or below while the pointer is on a row", () => {
    const { $, move, stop } = layout();
    // Exact middle of "Benachrichtigungen" (72 + 16) and one px either side: lit on that row, never on "Backups" above.
    expect([move(120, 87, $("notify-label")), move(120, 88, $("notify-label")), move(120, 89, $("notify-label"))]).toEqual([
      { lit: true, top: 72 },
      { lit: true, top: 72 },
      { lit: true, top: 72 },
    ]);
    // Near its top border the light sits on the hovered row itself.
    expect(move(120, 74, $("notify-label"))).toEqual({ lit: true, top: 72 });
    // Sweep down through all three rows: whenever a row is under the pointer, the light is on that row or off.
    for (let y = 0; y <= 104; y++) {
      const under = [0, 36, 72].find((top) => y >= top && y <= top + 32);
      const glow = move(120, y);
      if (under !== undefined) expect(glow.top === null || glow.top === under, `y=${y}`).toBe(true);
    }
    // In the 4 px gap the nearest row wins.
    expect(move(120, 33, $("menu")).top).toBe(0);
    expect(move(120, 35, $("menu")).top).toBe(36);
    stop();
  });

  it("treats a counter badge as part of its row", () => {
    const { $, move, stop } = layout();
    expect(glowStart($("badge"))).toBe($("hosts"));
    expect(glowStart($("hosts-icon"))).toBe($("hosts-icon"));
    // The badge overlays the right end of "Hosts" near the bottom border; the light stays on "Hosts", never "Backups".
    expect(move(236, 28, $("badge"))).toEqual({ lit: true, top: 0 });
    stop();
  });

  it("keeps settings rows apart the same way", () => {
    const { $, move, stop } = layout();
    // Centre line of "Darstellung" (240 + 18) lights its own row, not "Allgemein" above.
    expect([move(120, 258, $("s-look")).top, move(120, 257, $("s-look")).top, move(120, 242, $("s-look")).top]).toEqual([240, 240, 240]);
    stop();
  });

  it("still lights a real card border from just outside", () => {
    const { move, stop } = layout();
    // 6 px left of the card, pointer over the page background: soft outside light on the card.
    expect(move(394, 200, document.body)).toEqual({ lit: true, top: 0 });
    stop();
  });

  it("drops probed neighbours only for rows", () => {
    layout();
    const row = document.getElementById("notify")!;
    const other = document.getElementById("backups")!;
    const card = document.getElementById("card")!;
    expect(glowCandidates([row], [other, card, null])).toEqual([row]);
    expect(glowCandidates([card], [other, null])).toEqual([card, other]);
    expect(glowCandidates([], [other, card])).toEqual([other, card]);
  });
});
