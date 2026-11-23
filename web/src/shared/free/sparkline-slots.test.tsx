// Source: docklet-next/issues/582
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original fixed-slot bar proofs for M37 #582.

import { render } from "@testing-library/react";
import { expect, it } from "vitest";

import { Sparkline } from "./sparkline";
import { barSlots } from "./sparkline-geometry";

const rects = (container: HTMLElement) => [...container.querySelectorAll("rect")];

it("keeps the bar count fixed and fills missing slots with the mean of the real samples", () => {
  expect(barSlots([], 6)).toEqual([]);
  expect(barSlots([30], 4)).toEqual([
    { value: 30, placeholder: true }, { value: 30, placeholder: true }, { value: 30, placeholder: true }, { value: 30, placeholder: false },
  ]);
  const partial = barSlots([10, 20, 60], 5);
  expect(partial.map((b) => b.placeholder)).toEqual([true, true, false, false, false]);
  expect(partial[0]!.value).toBe(30);
  expect(barSlots([1, 2, 3, 4, 5, 6, 7], 5).map((b) => b.value)).toEqual([3, 4, 5, 6, 7]);
});

it("draws the same bar width from the first sample on, placeholders muted and real bars in the host colour", () => {
  const widths = [[42], [42, 50, 61], Array.from({ length: 60 }, (_, i) => i)].map((data) => {
    const { container, unmount } = render(<Sparkline data={data} type="bar" slots={60} color="#123456" domain={[0, 100]} />);
    const bars = rects(container);
    expect(bars.length).toBe(60);
    const real = bars.filter((b) => !b.hasAttribute("data-placeholder"));
    expect(real.length).toBe(Math.min(60, data.length));
    expect(real.every((b) => b.style.fill === "#123456")).toBe(true);
    expect(bars.filter((b) => b.hasAttribute("data-placeholder")).every((b) => b.style.fill === "var(--muted-foreground)")).toBe(true);
    const width = bars[0]!.getAttribute("width");
    unmount();
    return width;
  });
  expect(new Set(widths).size).toBe(1);
});

it("draws nothing before the first sample", () => {
  const { container } = render(<Sparkline data={[]} type="bar" slots={60} domain={[0, 100]} />);
  expect(rects(container).length).toBe(0);
});
