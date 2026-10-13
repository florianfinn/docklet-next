import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { visualStyleStore } from "@/shared/preferences/visual-style-store";

type Callback = (entries: { isIntersecting: boolean }[]) => void;

const ios: { cb: Callback }[] = [];
let pending = new Map<number, FrameRequestCallback>();
let nextId = 1;
let hidden = false;

function canvas(): HTMLCanvasElement {
  const el = document.createElement("canvas");
  el.getBoundingClientRect = () => ({ width: 200, height: 100 }) as DOMRect;
  el.getContext = (() =>
    new Proxy({}, { get: (_t, k) => (k === "getImageData" ? () => ({ data: [10, 20, 30, 255] }) : () => undefined), set: () => true })) as never;
  return el;
}

function setHidden(value: boolean) {
  hidden = value;
  document.dispatchEvent(new Event("visibilitychange"));
}

// Runs the queued animation frame callbacks once.
function runFrame(now = 1000) {
  const batch = [...pending.entries()];
  pending = new Map();
  for (const [, cb] of batch) cb(now);
}

const planned = () => pending.size;

describe("dot field loop lifecycle", () => {
  beforeEach(() => {
    vi.resetModules();
    pending = new Map();
    nextId = 1;
    hidden = false;
    ios.length = 0;
    Object.defineProperty(document, "hidden", { configurable: true, get: () => hidden });
    vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => {
      pending.set(nextId, cb);
      return nextId++;
    });
    vi.stubGlobal("cancelAnimationFrame", (id: number) => pending.delete(id));
    vi.stubGlobal("ResizeObserver", class { observe() {} disconnect() {} });
    vi.stubGlobal(
      "IntersectionObserver",
      class {
        constructor(cb: Callback) {
          ios.push({ cb });
        }
        observe() {}
        disconnect() {}
      },
    );
    document.documentElement.dataset.motion = "full";
    visualStyleStore.set((s) => ({ ...s, dotWave: true }));
  });

  afterEach(() => {
    vi.unstubAllGlobals();
    delete document.documentElement.dataset.motion;
  });

  async function mount(count = 1) {
    const { attachDotField } = await import("./dot-field");
    return Array.from({ length: count }, () => attachDotField(canvas()));
  }

  it("plans one frame while visible, none while hidden, one again when shown, also after repeated toggles", async () => {
    await mount();
    expect(planned()).toBe(1);
    setHidden(true);
    expect(planned()).toBe(0);
    setHidden(false);
    expect(planned()).toBe(1);
    setHidden(true);
    setHidden(false);
    expect(planned()).toBe(1);
  });

  it("stops after one frame when every field is out of view and restarts on re-entry", async () => {
    await mount();
    ios[0]?.cb([{ isIntersecting: false }]);
    expect(planned()).toBe(0);
    runFrame();
    expect(planned()).toBe(0);
    ios[0]?.cb([{ isIntersecting: true }]);
    expect(planned()).toBe(1);
  });

  it("keeps running while another field is visible", async () => {
    await mount(2);
    ios[0]?.cb([{ isIntersecting: false }]);
    expect(planned()).toBe(1);
    ios[1]?.cb([{ isIntersecting: false }]);
    expect(planned()).toBe(0);
  });

  it("cancels the loop when the last field unmounts", async () => {
    const [off] = await mount();
    off?.();
    expect(planned()).toBe(0);
  });
});
