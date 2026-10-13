import { render } from "@testing-library/react";
import { IntlProvider } from "use-intl";
import { afterEach, describe, expect, it, vi } from "vitest";

import { messages } from "@/app/i18n/messages";
import { GLOW_SELECTOR, glowCandidates, glowLayer, startPointerGlow } from "@/shared/lib/pointer-glow";
import { TOAST_DESIGNS } from "@/shared/preferences/ui-style-store";

import { ActionToastView } from "./ActionToastView";
import { sampleRun } from "./sample-run";

const css = import.meta.glob<string>(["/src/styles/notifications.css", "/src/styles/motion.css"], { query: "?raw", import: "default", eager: true });
const notifications = css["/src/styles/notifications.css"] ?? "";
const motion = css["/src/styles/motion.css"] ?? "";
const block = (text: string, selector: string) => new RegExp(`${selector.replace(/[.*+?^${}()|[\]\\]/g, "\\$&")} \\{([^}]*)\\}`).exec(text)?.[1] ?? "";

afterEach(() => {
  vi.restoreAllMocks();
  document.body.innerHTML = "";
});

describe("notification frame", () => {
  it("keeps a neutral frame in every state; no status-coloured outline, not even on hover", () => {
    expect(block(notifications, ".action-toast .toast-card")).toMatch(/border-color: var\(--border\);/);
    expect(block(notifications, ".action-toast .toast-card")).not.toMatch(/--tone/);
    expect(/\.action-toast:hover \.toast-card \{[^}]*border-color/.test(notifications)).toBe(false);
    expect(block(notifications, "[data-sonner-toaster] [data-sonner-toast][data-styled=\"true\"]")).toMatch(/border: 1px solid var\(--border\);/);
    // The overlay's layer is set per target, no fixed layer below the toaster.
    expect(block(motion, '[data-slot="pointer-glow"]')).toMatch(/z-index: 1;/);
  });

  it("registers every toast design for the edge light, also when the run succeeded", () => {
    const run = { ...sampleRun(new Date().toISOString()), status: "succeeded" as const, progress: 100 };
    for (const design of TOAST_DESIGNS) {
      const { container, unmount } = render(
        <IntlProvider locale="de" messages={messages} timeZone="Europe/Berlin">
          <ActionToastView run={run} design={design} detail="full" now={Date.now()} major onClose={() => {}} onOpen={() => {}} />
        </IntlProvider>,
      );
      const card = container.querySelector<HTMLElement>(".toast-card");
      expect([design, card?.matches(GLOW_SELECTOR), card?.getAttribute("style")]).toEqual([design, true, null]);
      unmount();
    }
  });
});

describe("edge light layers", () => {
  it("reads the stacking layer and ignores neighbours below the hovered layer", () => {
    document.body.innerHTML = `<div id="page"><div data-slot="card" id="card"></div></div><ol style="position:fixed;z-index:100"><li style="position:absolute;z-index:3"><div class="toast-card" id="toast"></div></li></ol>`;
    const $ = (id: string) => document.getElementById(id)!;
    expect([glowLayer($("card")), glowLayer($("toast"))]).toEqual([0, 100]);
    expect(glowCandidates([$("toast")], [$("card"), null], glowLayer)).toEqual([$("toast")]);
    expect(glowCandidates([$("card")], [$("toast")], glowLayer)).toEqual([$("card"), $("toast")]);
  });

  it("paints a toast's light one layer above the toaster, a page card's light below anything covering it", () => {
    document.body.innerHTML = `<div data-slot="card" id="card"></div><ol style="position:fixed;z-index:100"><li><div class="toast-card" id="toast"></div></li></ol>`;
    const card = document.getElementById("card")!;
    const toast = document.getElementById("toast")!;
    vi.spyOn(card, "getBoundingClientRect").mockReturnValue(new DOMRect(0, 0, 300, 200));
    vi.spyOn(toast, "getBoundingClientRect").mockReturnValue(new DOMRect(600, 0, 360, 120));
    Object.defineProperty(document, "elementFromPoint", { configurable: true, value: () => document.body });
    const frames: FrameRequestCallback[] = [];
    vi.spyOn(window, "requestAnimationFrame").mockImplementation((cb) => frames.push(cb));
    const stop = startPointerGlow();
    const overlay = document.querySelector<HTMLElement>('[data-slot="pointer-glow"]')!;
    const move = (target: Element, x: number, y: number) => {
      target.dispatchEvent(new PointerEvent("pointermove", { bubbles: true, clientX: x, clientY: y, pointerType: "mouse" }));
      frames.splice(0).forEach((cb) => cb(0));
      return [Number(overlay.style.opacity) > 0, overlay.style.zIndex];
    };
    expect(move(toast, 605, 60)).toEqual([true, "101"]);
    expect(move(card, 5, 100)).toEqual([true, "1"]);
    stop();
  });
});
