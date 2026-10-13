import { act, screen } from "@testing-library/react";
import { toast } from "sonner";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import css from "@/styles/notifications.css?raw";
import { renderWithApp } from "@/test/render";

import { AppToaster } from "./AppToaster";
import { NOTICE_LIFE } from "./notice-life";

beforeEach(() => vi.useFakeTimers());
afterEach(() => {
  act(() => void toast.dismiss());
  vi.useRealTimers();
});

const tick = (ms: number) => act(async () => void (await vi.advanceTimersByTimeAsync(ms)));
const notice = (title: string) => screen.getByText(title).closest<HTMLElement>("[data-sonner-toast]");
const endBar = (el: HTMLElement, animationName: string) => {
  const e = new Event("animationend", { bubbles: true });
  Object.defineProperty(e, "animationName", { value: animationName });
  act(() => void el.dispatchEvent(e));
};

describe("notice lifetime", () => {
  it("lives exactly as long as its countdown bar, so pausing the bar pauses the notice", async () => {
    renderWithApp(<AppToaster />);
    act(() => void toast.success("Gespeichert"));
    await tick(50);
    const el = notice("Gespeichert");
    expect(el !== null).toBe(true);

    // Without the bar ending, no sonner timer closes it (a paused bar keeps it, however long).
    await tick(NOTICE_LIFE.error * 3);
    expect(screen.queryByText("Gespeichert") !== null).toBe(true);

    endBar(el as HTMLElement, "pop-in");
    await tick(500);
    expect(screen.queryByText("Gespeichert") !== null).toBe(true);

    endBar(el as HTMLElement, "notice-life");
    await tick(500);
    expect(screen.queryByText("Gespeichert") === null).toBe(true);
  });

  it("pauses bar and lifetime together for hover, keyboard focus, Alt+T and a hidden page; errors last longer", () => {
    const pause = css.slice(0, css.indexOf("animation-play-state: paused !important"));
    for (const selector of [
      "[data-sonner-toaster]:hover [data-sonner-toast]",
      "[data-sonner-toaster]:focus-within [data-sonner-toast]",
      "[data-page-hidden] [data-sonner-toast]",
      "[data-page-hidden] .action-toast",
    ])
      expect(pause.includes(selector), selector).toBe(true);
    expect(css.includes(`--notice-life: ${NOTICE_LIFE.default}ms`) && css.includes(`--notice-life: ${NOTICE_LIFE.error}ms`)).toBe(true);
    // Loading notices never expire; reduced motion keeps a stepped timer instead of sonner's "animation: none".
    expect(css.includes(':not([data-swipe-out="true"], [data-type="loading"])')).toBe(true);
    expect(/prefers-reduced-motion[^}]*notice-life var\(--notice-life\) var\(--notice-steps\)/.test(css)).toBe(true);
  });

  it("keeps modal dialogs modal: toasts sit under any open overlay, without pointer or focus overrides", () => {
    expect(css.includes("pointer-events")).toBe(false);
    const rule = css.slice(css.indexOf("body:has("), css.indexOf("@keyframes notice-life"));
    for (const slot of ["dialog-overlay", "alert-dialog-overlay", "sheet-overlay"]) expect(rule.includes(`[data-slot="${slot}"]`), slot).toBe(true);
    expect(/z-index: (\d+)/.exec(rule)?.[1]).toBe("40");
  });

  it("marks the page as hidden while the tab is in the background", () => {
    renderWithApp(<AppToaster />);
    Object.defineProperty(document, "hidden", { configurable: true, get: () => true });
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    expect(document.documentElement.hasAttribute("data-page-hidden")).toBe(true);
    Object.defineProperty(document, "hidden", { configurable: true, get: () => false });
    act(() => void document.dispatchEvent(new Event("visibilitychange")));
    expect(document.documentElement.hasAttribute("data-page-hidden")).toBe(false);
  });
});
