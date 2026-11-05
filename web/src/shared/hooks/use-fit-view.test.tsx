// Source: docklet-next/issues/266
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original remeasure-key guard for the measured overview view.

import { render } from "@testing-library/react";
import { afterEach, expect, it } from "vitest";
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
