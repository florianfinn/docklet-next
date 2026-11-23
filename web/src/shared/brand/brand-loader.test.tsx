// Source: docklet-next/issues/580
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original loader lifecycle proofs for M37 #580.

import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";

import { BrandLoader, LOADER_CYCLE_MS } from "./BrandLoader";

interface FakeAnimation { cancelled: boolean; cancel(): void; finished: Promise<void> }
let started: FakeAnimation[] = [];
let reduced = false;
let mediaListeners: (() => void)[] = [];
const native = Object.getOwnPropertyDescriptor(Element.prototype, "animate");

beforeEach(() => {
  vi.useFakeTimers();
  started = []; reduced = false; mediaListeners = [];
  Object.defineProperty(Element.prototype, "animate", { configurable: true, value: function animate() { const a: FakeAnimation = { cancelled: false, cancel() { this.cancelled = true; }, finished: new Promise<void>(() => {}) }; started.push(a); return a; } });
  vi.stubGlobal("matchMedia", (query: string) => ({ get matches() { return query.includes("reduce") && reduced; }, addEventListener: (_: string, fn: () => void) => mediaListeners.push(fn), removeEventListener: () => {} }));
});

afterEach(() => {
  vi.useRealTimers();
  vi.unstubAllGlobals();
  if (native) Object.defineProperty(Element.prototype, "animate", native);
  else delete (Element.prototype as { animate?: unknown }).animate;
});

const live = () => started.filter((a) => !a.cancelled).length;

it("loops the logo animation and leaves nothing running after unmount", () => {
  const { container, unmount } = render(<BrandLoader size={48} />);
  expect(container.querySelector('[data-slot="brand-loader"]')?.hasAttribute("data-animated")).toBe(true);
  expect(container.querySelector('[data-slot="brand-logo"] svg') !== null).toBe(true);
  const first = started.length;
  expect(first > 5).toBe(true);
  act(() => { vi.advanceTimersByTime(LOADER_CYCLE_MS); });
  expect(started.length > first).toBe(true);
  expect(started.slice(0, first).every((a) => a.cancelled)).toBe(true);
  unmount();
  expect(live()).toBe(0);
  const after = started.length;
  act(() => { vi.advanceTimersByTime(LOADER_CYCLE_MS * 3); });
  expect(started.length).toBe(after);
});

it("shows the static logo with reduced motion and stops a running loop when the preference changes", () => {
  reduced = true;
  const still = render(<BrandLoader />);
  expect(started.length).toBe(0);
  expect(still.container.querySelector('[data-slot="brand-loader"]')?.hasAttribute("data-animated")).toBe(false);
  still.unmount();

  reduced = false;
  render(<BrandLoader />);
  expect(live() > 0).toBe(true);
  reduced = true;
  act(() => { mediaListeners.forEach((fn) => fn()); });
  expect(live()).toBe(0);
});

it("stops when motion is switched off without unmounting", () => {
  const view = render(<BrandLoader motion />);
  expect(live() > 0).toBe(true);
  view.rerender(<BrandLoader motion={false} />);
  expect(live()).toBe(0);
  const count = started.length;
  act(() => { vi.advanceTimersByTime(LOADER_CYCLE_MS * 2); });
  expect(started.length).toBe(count);
});
