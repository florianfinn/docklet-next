import { describe, expect, it, vi } from "vitest";

import { edgeGlow, GLOW_REACH, glowTarget, roundedEdgePoint, startPointerGlow } from "@/shared/lib/pointer-glow";

import { contrast, hexToOklch } from "./color";
import { PALETTE_PRESETS, paletteTokens, type Palette } from "./palette";

const oklchL = (css: string) => Number(/oklch\(([\d.]+)/.exec(css)?.[1]);
const oklchH = (css: string) => Number(/oklch\([\d.]+ [\d.]+ ([\d.]+)/.exec(css)?.[1]);

describe("palette roles", () => {
  it("drives buttons, navigation hover and controls from primary and accents, glows and skeletons from secondary", () => {
    const base = PALETTE_PRESETS[0]!.palette;
    for (const theme of ["dark", "light"] as const) {
      const a = paletteTokens(base, theme).vars;
      const p = paletteTokens({ ...base, primary: "#00a37a" }, theme).vars;
      const s = paletteTokens({ ...base, secondary: "#00c2e0" }, theme).vars;
      const primaryOwned = ["--control", "--control-text", "--secondary", "--secondary-hover", "--accent", "--sidebar-accent"];
      const secondaryOwned = ["--brand-accent", "--brand-accent-text", "--chart-1", "--highlight", "--skeleton"];
      // A primary change moves every primary-owned variable and no secondary-owned one, and vice versa.
      expect(primaryOwned.every((v) => a[v] !== p[v]) && secondaryOwned.every((v) => a[v] === p[v]), `${theme} primary`).toBe(true);
      expect(secondaryOwned.every((v) => a[v] !== s[v]) && primaryOwned.every((v) => a[v] === s[v]), `${theme} secondary`).toBe(true);
      // Tinted button fill and hover surfaces carry the primary hue; hover is lighter in dark and darker in light schemes.
      expect(Math.abs(oklchH(p["--secondary"] ?? "") - hexToOklch("#00a37a").h) < 1 && Math.abs(oklchH(p["--accent"] ?? "") - hexToOklch("#00a37a").h) < 1).toBe(true);
      const step = oklchL(p["--secondary-hover"] ?? "") - oklchL(p["--secondary"] ?? "");
      expect(theme === "dark" ? step > 0.03 : step < -0.03).toBe(true);
      expect(Math.abs(oklchH(s["--skeleton"] ?? "") - hexToOklch("#00c2e0").h) < 1).toBe(true);
    }
  });

  it("keeps labels on the tinted button fill readable for presets and random palettes", () => {
    let seed = 11;
    const rand = () => ((seed = (seed * 1664525 + 1013904223) % 4294967296) / 4294967296);
    const hex = () => `#${Math.floor(rand() * 0xffffff).toString(16).padStart(6, "0")}`;
    const palettes: Palette[] = [...PALETTE_PRESETS.map((p) => p.palette), ...Array.from({ length: 200 }, () => ({ primary: hex(), secondary: hex(), surface: hex() }))];
    for (const p of palettes) for (const theme of ["dark", "light"] as const) expect(paletteTokens(p, theme).report.secondaryLabel >= 4.5, `${JSON.stringify(p)}/${theme}`).toBe(true);
    expect(contrast(hexToOklch("#ffffff"), hexToOklch("#000000")) > 20).toBe(true);
  });

  it("styles controls with --control and their halos, the edge light and skeletons with secondary variables", () => {
    const files = import.meta.glob<string>(["/src/styles/controls.css", "/src/styles/motion.css", "/src/shared/ui/skeleton.tsx"], { query: "?raw", import: "default", eager: true });
    const css = (path: string) => files[path] ?? "";
    const controls = css("/src/styles/controls.css");
    const motion = css("/src/styles/motion.css");
    const shadows = [...controls.matchAll(/box-shadow:[^;]*;/g)].map((m) => m[0]).filter((d) => d.includes("calc(") && d.includes("--glow"));
    expect(shadows.filter((d) => d.includes("var(--highlight)")).length >= 5 && shadows.every((d) => !d.includes("var(--control)"))).toBe(true);
    expect(/\.seg-indicator \{[^}]*var\(--control\)/.test(controls)).toBe(true);
    expect(/--glow-color:[^;]*var\(--highlight\)/.test(motion)).toBe(true);
    expect(css("/src/shared/ui/skeleton.tsx").includes("bg-skeleton")).toBe(true);
  });
});

describe("pointer glow", () => {
  it("lights only near the border, inside or just outside, and never the centre of a large card", () => {
    // 600 × 400 card with a 14 px radius.
    const at = (x: number, y: number) => edgeGlow(x, y, 600, 400, 14);
    expect(at(300, 200).strength).toBe(0);
    expect(at(300, 400 - GLOW_REACH.inside - 1).strength).toBe(0);
    expect(at(300, 395).strength > 0.5 && at(300, 395).y === 400).toBe(true);
    // Just outside: a faint shimmer, weaker than the same distance inside; nothing beyond the outside reach.
    const outside = at(300, 405).strength;
    expect(outside > 0 && outside < at(300, 395).strength).toBe(true);
    expect(at(300, 400 + GLOW_REACH.outside + 1).strength).toBe(0);
    // Closer to the border means more light.
    expect(at(300, 398).strength > at(300, 390).strength).toBe(true);
  });

  it("moves continuously around a rounded bottom-right corner instead of jumping from the bottom to the right edge", () => {
    const w = 600;
    const h = 400;
    const r = 14;
    let prev = edgeGlow(w - 30, h - 4, w, h, r);
    let maxStep = 0;
    // Sweep along a path 4 px inside the border, from the bottom edge around the corner up the right edge.
    for (let t = 1; t <= 120; t++) {
      const a = (t / 120) * (Math.PI / 2);
      const px = w - 30 + Math.min(26, 26 * Math.sin(a) + (t > 60 ? 0 : 0));
      const py = h - 4 - 26 * (1 - Math.cos(a));
      const g = edgeGlow(px, py, w, h, r);
      maxStep = Math.max(maxStep, Math.hypot(g.x - prev.x, g.y - prev.y));
      prev = g;
    }
    expect(maxStep < 3).toBe(true);
    // Exactly on the corner diagonal the centre sits on the arc, between both edges.
    const diag = roundedEdgePoint(w - 5, h - 5, w, h, r);
    expect(diag.x > w - r && diag.x < w && diag.y > h - r && diag.y < h).toBe(true);
    expect(Math.abs(Math.hypot(diag.x - (w - r), diag.y - (h - r)) - r) < 1e-9).toBe(true);
    // Sharp corner: the light centre still rounds the corner within the visible reach.
    const a = edgeGlow(w - 9, h - 10, w, h, 0);
    const b = edgeGlow(w - 10, h - 9, w, h, 0);
    expect(Math.hypot(a.x - b.x, a.y - b.y) < 2 * Math.SQRT2).toBe(true);
  });

  it("measures straight edges, corners and the outside with signed distances", () => {
    expect(roundedEdgePoint(8, 50, 200, 100, 0)).toEqual({ x: 0, y: 50, distance: -8 });
    expect(roundedEdgePoint(120, 93, 200, 100, 10)).toEqual({ x: 120, y: 100, distance: -7 });
    expect(roundedEdgePoint(250, 50, 200, 100, 10)).toEqual({ x: 200, y: 50, distance: 50 });
    // Outside the top-right corner the nearest point lies on the corner arc.
    const p = roundedEdgePoint(210, -10, 200, 100, 10);
    expect(p.distance > 0 && p.x > 190 && p.y < 10).toBe(true);
  });

  it("targets buttons and cards, never nodes of the topology graph", () => {
    document.body.innerHTML = '<div data-slot="card"><span id="in-card"></span></div><div class="react-flow"><button data-slot="button" id="flow-btn"></button></div><p id="plain"></p>';
    expect(glowTarget(document.getElementById("in-card"))?.dataset.slot).toBe("card");
    expect(glowTarget(document.getElementById("flow-btn"))).toBe(null);
    expect(glowTarget(document.getElementById("plain"))).toBe(null);
  });

  it("follows the mouse with one overlay per frame, ignores touch and cleans up", () => {
    document.body.innerHTML = '<button data-slot="button" id="b">x</button>';
    const button = document.getElementById("b")!;
    vi.spyOn(button, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 120, 40));
    const frames: FrameRequestCallback[] = [];
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => frames.push(cb));
    const add = vi.spyOn(document, "addEventListener");
    const remove = vi.spyOn(document, "removeEventListener");
    const stop = startPointerGlow();
    const overlay = document.querySelector<HTMLElement>('[data-slot="pointer-glow"]');
    expect(overlay !== null && add.mock.calls.filter(([type]) => type === "pointermove").length === 1).toBe(true);

    const move = (pointerType: string, clientX = 60, clientY = 4) => button.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, clientX, clientY, pointerType }));
    move("touch");
    expect(frames.length).toBe(0);
    move("mouse");
    move("mouse");
    // Two moves, one scheduled frame; painting writes CSS variables only.
    expect(frames.length).toBe(1);
    frames[0]?.(0);
    expect([Number(overlay?.style.opacity) > 0.5, overlay?.style.getPropertyValue("--gy")]).toEqual([true, "0px"]);
    // Border ring only: no interior fill part that could light the surface.
    expect([...(overlay?.children ?? [])].map((c) => (c as HTMLElement).dataset.part)).toEqual(["ring"]);
    // Pointer in the middle of a wide card-sized target: no light at all.
    vi.spyOn(button, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 600, 400));
    move("mouse", 300, 200);
    frames[1]?.(0);
    expect(overlay?.style.opacity).toBe("0");

    stop();
    expect(document.querySelector('[data-slot="pointer-glow"]') === null && remove.mock.calls.some(([type]) => type === "pointermove")).toBe(true);
    vi.restoreAllMocks();
  });
});
