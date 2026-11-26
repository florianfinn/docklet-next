// Source: docklet-next/issues/266
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original remeasure-key guard for the measured overview view. Web font completion refits from the widest view and keeps mounted rows while a popup is open.

import { act, render } from "@testing-library/react";
import type { Ref } from "react";
import { afterEach, expect, it } from "vitest";
import { useExclusivePopup } from "@/shared/lib/popup-coordinator";
import { useFitView } from "./use-fit-view";

let width = 300;
const original = HTMLElement.prototype.getBoundingClientRect;
const client = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth");
afterEach(() => { HTMLElement.prototype.getBoundingClientRect = original; if (client) Object.defineProperty(HTMLElement.prototype, "clientWidth", client); });

function Probe({ remeasure }: { remeasure: string }) {
  const { view, ref } = useFitView<HTMLDivElement>("data", undefined, remeasure);
  return <section><div ref={ref} data-view={view} /></section>;
}

it("a new remeasure key measures again from the current view without resetting it", () => {
  HTMLElement.prototype.getBoundingClientRect = function () { return { width: this.tagName === "DIV" ? width : 0 } as DOMRect; };
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return 330; } });
  const { container, rerender } = render(<Probe remeasure="a" />);
  const view = () => container.querySelector("div")!.getAttribute("data-view");
  expect(view()).toBe("table");
  // Wider content (for example a cleared filter) only steps down once the key changes.
  width = 1000;
  rerender(<Probe remeasure="a" />);
  expect(view()).toBe("table");
  rerender(<Probe remeasure="b" />);
  expect(view() === "table").toBe(false);
});

let widths: Record<string, number> = {};
function Swapping() {
  const { view, ref } = useFitView<HTMLElement>("data");
  return <section>{view === "cards" ? <ul ref={ref as Ref<HTMLUListElement>} data-view={view} /> : <div ref={ref as Ref<HTMLDivElement>} data-view={view} />}</section>;
}
function Popup() {
  const [open, setOpen] = useExclusivePopup();
  return <button type="button" data-open={open} onClick={() => setOpen(!open)}>popup</button>;
}

/** Fallback font: only cards fit 600 px. The loaded font narrows the table so it fits again. */
function fontSetup() {
  widths = { table: 700, condensed: 650, cards: 600 };
  HTMLElement.prototype.getBoundingClientRect = function () { return { width: widths[this.getAttribute("data-view") ?? ""] ?? 0 } as DOMRect; };
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return 600; } });
  const fonts = new EventTarget();
  Object.defineProperty(document, "fonts", { configurable: true, value: fonts });
  const { container } = render(<><Popup /><Swapping /></>);
  const view = () => container.querySelector("[data-view]")!;
  const load = () => { widths = { table: 590, condensed: 560, cards: 600 }; act(() => { fonts.dispatchEvent(new Event("loadingdone")); }); };
  const toggle = () => act(() => { container.querySelector("button")!.click(); });
  return { view, load, toggle };
}
afterEach(() => { delete (document as { fonts?: unknown }).fonts; });

it("web font completion measures again from the widest view, so a table that now fits returns", () => {
  const { view, load } = fontSetup();
  expect(view().getAttribute("data-view")).toBe("cards");
  load();
  expect(view().getAttribute("data-view")).toBe("table");
});

it("while a popup is open, web font completion keeps the mounted rows and resets once it closes", () => {
  const { view, load, toggle } = fontSetup();
  const cards = view();
  toggle();
  load();
  // An open hover card inside the cards survives only if the subtree is the same node.
  expect([view() === cards, cards.isConnected]).toEqual([true, true]);
  toggle();
  expect(view().getAttribute("data-view")).toBe("table");
});
