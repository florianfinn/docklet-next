import { describe, expect, it } from "vitest";

// Popup surfaces keep Radix's standard "…-content" slot: styles that must not touch an opening or closing
// surface (scroll cues replacing its exit animation) key on it. Feature markers use their own attribute.
const sources = import.meta.glob<string>(["/src/**/*.tsx", "!/src/shared/ui/**"], { query: "?raw", import: "default", eager: true });
const scrollCss = import.meta.glob<string>("/src/styles/scroll.css", { query: "?raw", import: "default", eager: true })["/src/styles/scroll.css"]!;
const CONTENT = /<((?:Alert)?Dialog|Sheet|Popover|DropdownMenu|ContextMenu|Select|HoverCard|Tooltip|Menubar)(?:Sub)?Content\b(?:=>|[^>])*?\sdata-slot=/s;

describe("popup surfaces", () => {
  it("never replace the content slot of a Radix surface", () => {
    expect(CONTENT.test('<ContextMenuContent onCloseAutoFocus={(e) => e.preventDefault()} data-slot="segment-menu">')).toBe(true);
    expect(Object.entries(sources).filter(([, code]) => CONTENT.test(code)).map(([path]) => path)).toEqual([]);
  });

  it("keep scroll cues off surfaces and closing elements in every cue rule", () => {
    const rules = scrollCss.match(/:not\([^{]*\{/g) ?? [];
    expect(rules.length).toBe(3);
    for (const marker of ['[data-slot$="-content"]', '[role="dialog"]', '[role="menu"]', '[data-state="closed"]']) expect(rules.every((r) => r.includes(marker)), marker).toBe(true);
  });
});
