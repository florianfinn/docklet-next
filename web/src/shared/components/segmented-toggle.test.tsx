import { fireEvent, render, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { TooltipProvider } from "@/shared/ui/tooltip";

import { SegmentedToggle } from "./segmented-toggle";

const controls = import.meta.glob<string>("/src/styles/controls.css", { query: "?raw", import: "default", eager: true })["/src/styles/controls.css"] ?? "";

const SEG_WIDTH = 64;
const saved = new Map<string, PropertyDescriptor | undefined>();

/** happy-dom has no layout: segment boxes are laid out side by side, 64 × 28 px each. */
beforeEach(() => {
  const index = (el: HTMLElement) => [...(el.parentElement?.querySelectorAll(":scope > .seg-item") ?? [])].indexOf(el);
  const box: Record<string, (el: HTMLElement) => number> = {
    offsetWidth: (el) => (el.classList.contains("seg-item") ? SEG_WIDTH : 200),
    offsetHeight: () => 28,
    offsetLeft: (el) => (el.classList.contains("seg-item") ? 3 + index(el) * SEG_WIDTH : 0),
    offsetTop: () => 3,
  };
  for (const [key, get] of Object.entries(box)) {
    saved.set(key, Object.getOwnPropertyDescriptor(HTMLElement.prototype, key));
    Object.defineProperty(HTMLElement.prototype, key, {
      configurable: true,
      get(this: HTMLElement) {
        return get(this);
      },
    });
  }
});

afterEach(() => {
  for (const [key, desc] of saved) if (desc) Object.defineProperty(HTMLElement.prototype, key, desc);
  saved.clear();
});

function Harness({ initial = "all" }: { initial?: string }) {
  const [value, setValue] = useState(initial);
  return (
    <TooltipProvider>
      <SegmentedToggle value={value} onChange={setValue} label="Filter" options={["all", "running", "stopped"].map((v) => ({ value: v, label: v }))} />
    </TooltipProvider>
  );
}

describe("segmented toggle indicator", () => {
  it("is placed without a transition on mount and slides only after a deliberate change", () => {
    const records: MutationRecord[] = [];
    const observer = new MutationObserver((list) => records.push(...list));
    observer.observe(document.body, { subtree: true, attributes: true, attributeFilter: ["data-instant"] });
    const { unmount } = render(<Harness initial="running" />);
    records.push(...observer.takeRecords());
    const root = document.querySelector<HTMLElement>('[data-slot="segmented-toggle"]')!;
    // First placement (also what a new page shows): transitions off while the position is set, then back on.
    expect([records.length, root.hasAttribute("data-instant"), root.hasAttribute("data-measured"), root.style.getPropertyValue("--seg-x")]).toEqual([2, false, true, "67px"]);

    records.length = 0;
    fireEvent.click(screen.getByRole("radio", { name: "stopped" }));
    records.push(...observer.takeRecords());
    // A chosen value slides: no instant placement, new position written.
    expect([records.length, root.style.getPropertyValue("--seg-x")]).toEqual([0, "131px"]);
    unmount();

    // Remounting (route change) never animates from the old or the index position.
    render(<Harness initial="stopped" />);
    records.push(...observer.takeRecords());
    expect([records.length, document.querySelector('[data-slot="segmented-toggle"]')?.hasAttribute("data-instant")]).toEqual([2, false]);
    observer.disconnect();
  });

  it("glides without overshoot and disables transitions while placing instantly", () => {
    const indicatorRules = [...controls.matchAll(/([^{}]*\.seg-indicator[^{}]*)\{([^}]*)\}/g)].map(([, sel, body]) => ({ sel: sel!.trim(), body: body! }));
    const transitions = indicatorRules.filter((r) => /transition:/.test(r.body));
    expect(transitions.length >= 2 && transitions.every((r) => !r.body.includes("--ease-spring"))).toBe(true);
    expect(/\.seg-item svg \{[^}]*transition: transform var\(--dur\) var\(--ease-out\)/.test(controls)).toBe(true);
    expect(indicatorRules.some((r) => r.sel.includes(".seg[data-instant][data-measured] .seg-indicator") && /transition: none/.test(r.body))).toBe(true);
    // Reduced motion still only fades colours.
    expect(/\[data-motion="reduced"\] \.seg\[data-measured\] \.seg-indicator/.test(controls)).toBe(true);
  });
});
