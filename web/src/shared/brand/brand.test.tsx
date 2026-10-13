import { act, render } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { BrandLogo } from "./BrandLogo";
import { replayBrandEntrance, resetBrandEntrances } from "./brand-entrance";
import { motionPlan } from "./vendor/logo-motion.js";
import { FAVORITE_LOGO } from "./vendor/logo-svg.js";

const vendored = import.meta.glob<string>("./vendor/*.js", { query: "?raw", import: "default", eager: true });
const manifest = import.meta.glob<string>("./vendor/PROVENANCE.md", { query: "?raw", import: "default", eager: true })["./vendor/PROVENANCE.md"] ?? "";

async function sha256(text: string): Promise<string> {
  const digest = await crypto.subtle.digest("SHA-256", new TextEncoder().encode(text));
  return [...new Uint8Array(digest)].map((b) => b.toString(16).padStart(2, "0")).join("");
}

let animate: ReturnType<typeof vi.fn>;
beforeEach(() => {
  resetBrandEntrances();
  animate = vi.fn(() => ({ cancel: vi.fn(), finish: vi.fn(), finished: Promise.resolve() }));
  (Element.prototype as unknown as { animate: unknown }).animate = animate;
});
afterEach(() => {
  delete (Element.prototype as unknown as { animate?: unknown }).animate;
});

describe("vendored logo modules", () => {
  // logo-next's own test compares these copies with its source files.
  it("are listed in PROVENANCE.md with the SHA-256 of their content", async () => {
    const files = Object.entries(vendored).map(([path, text]) => [path.slice("./vendor/".length), text] as const);
    expect(files.length).toBe(12);
    for (const [name, text] of files) {
      expect(manifest.includes(`| \`${name}\` | \`logo-next/src/${name}\` | \`${await sha256(text)}\` |`), `${name} in PROVENANCE.md`).toBe(true);
    }
  });

  it("plays relay with the mark settled before the letters and no recolour afterwards", () => {
    const plan = motionPlan("stack", "dropcap", 1900, { style: "relay" });
    const markEnd = Math.max(...plan.filter((s) => s.selector.includes("data-part")).map((s) => s.delay + s.duration));
    const letters = plan.filter((s) => s.group === "letters");
    expect([markEnd <= Math.min(...letters.map((s) => s.delay)), plan.some((s) => JSON.stringify(s.keyframes).includes("color-mix"))]).toEqual([true, false]);
    expect({ ...FAVORITE_LOGO }).toEqual({ candidate: "stack", variant: "dropcap", tone: "duo", motionStyle: "relay" });
  });
});

describe("BrandLogo", () => {
  it("renders the favourite as inline SVG: mark as D, OCKLET and NEXT, no image or frame", () => {
    const { container } = render(<BrandLogo />);
    const svg = container.querySelector("svg.dln-logo")!;
    expect([svg.getAttribute("data-variant"), svg.getAttribute("data-tone"), svg.getAttribute("aria-label"), container.querySelectorAll(".dln-letter").length, container.querySelectorAll("img, iframe").length]).toEqual(["dropcap", "duo", "Docklet Next", 10, 0]);
    expect(container.querySelector('[data-slot="brand-logo"]')?.classList.contains("brand-logo")).toBe(true);
  });

  it("plays the entrance once per slot and page load, not again on remount, and not at all without motion", () => {
    const first = render(<BrandLogo entranceSlot="sidebar" decorative />);
    const once = animate.mock.calls.length;
    expect(once > 0).toBe(true);
    first.unmount();
    render(<BrandLogo entranceSlot="sidebar" decorative />);
    expect(animate.mock.calls.length).toBe(once);
    // An explicit replay plays again.
    act(() => replayBrandEntrance());
    expect(animate.mock.calls.length > once).toBe(true);

    resetBrandEntrances();
    animate.mockClear();
    render(<BrandLogo entranceSlot="still" motion={false} decorative />);
    expect(animate.mock.calls.length).toBe(0);
  });

  it("hides a decorative logo from assistive technology", () => {
    const { container } = render(<BrandLogo variant="mark" decorative />);
    const svg = container.querySelector("svg")!;
    expect([svg.getAttribute("aria-hidden"), svg.hasAttribute("role"), svg.getAttribute("data-variant")]).toEqual(["true", false, "mark"]);
  });
});
