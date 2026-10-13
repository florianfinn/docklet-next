import { afterEach, describe, expect, it } from "vitest";

import { cueOf, installScrollCueFallback } from "./scroll-cue";

const metrics = (el: HTMLElement, m: Partial<Record<"scrollHeight" | "clientHeight" | "scrollWidth" | "clientWidth", number>>) => {
  for (const [k, v] of Object.entries(m)) Object.defineProperty(el, k, { configurable: true, value: v });
};
const region = (cls: string, m: Parameters<typeof metrics>[1]) => {
  const el = document.createElement("div");
  el.className = cls;
  metrics(el, m);
  document.body.append(el);
  return el;
};
const frame = () => new Promise((r) => requestAnimationFrame(() => r(undefined)));

afterEach(() => {
  document.body.innerHTML = "";
});

describe("scroll cue fallback", () => {
  it("fades only toward further content and never on a region that fits", () => {
    const m = { scrollTop: 0, scrollLeft: 0, scrollHeight: 600, clientHeight: 200, scrollWidth: 100, clientWidth: 100 };
    expect([cueOf(m, false), cueOf({ ...m, scrollTop: 10 }, false), cueOf({ ...m, scrollTop: 400 }, false), cueOf(m, true), cueOf({ ...m, scrollHeight: 200 }, false)]).toEqual([
      { start: 0, end: 24 },
      { start: 10, end: 24 },
      { start: 24, end: 0 },
      undefined,
      undefined,
    ]);
  });

  it("marks overflowing regions, follows their scrolling and skips surfaces and closing elements", async () => {
    const list = region("overflow-y-auto", { scrollHeight: 900, clientHeight: 300 });
    const fits = region("overflow-y-auto", { scrollHeight: 300, clientHeight: 300 });
    const wide = region("overflow-x-auto", { scrollWidth: 800, clientWidth: 300, scrollHeight: 40, clientHeight: 40 });
    const dialog = region("overflow-y-auto", { scrollHeight: 900, clientHeight: 300 });
    dialog.setAttribute("role", "dialog");
    const stop = installScrollCueFallback(document, true);
    await frame();
    expect([list.dataset.cue, fits.hasAttribute("data-cue"), wide.dataset.cue, dialog.hasAttribute("data-cue"), document.documentElement.hasAttribute("data-scroll-cue-js")]).toEqual(["y", false, "x", false, true]);
    expect([list.style.getPropertyValue("--cue-start"), list.style.getPropertyValue("--cue-end")]).toEqual(["0px", "24px"]);

    list.scrollTop = 600;
    list.dispatchEvent(new Event("scroll"));
    expect([list.style.getPropertyValue("--cue-start"), list.style.getPropertyValue("--cue-end")]).toEqual(["24px", "0px"]);

    // A closing element drops its cue so its exit animation is untouched.
    list.setAttribute("data-state", "closed");
    await frame();
    await frame();
    expect(list.hasAttribute("data-cue")).toBe(false);

    stop();
    expect(document.documentElement.hasAttribute("data-scroll-cue-js")).toBe(false);
  });

  it("updates when content grows without the region resizing (followed logs)", async () => {
    const log = region("overflow-auto", { scrollHeight: 200, clientHeight: 200 });
    const stop = installScrollCueFallback(document, true);
    await frame();
    expect(log.hasAttribute("data-cue")).toBe(false);
    metrics(log, { scrollHeight: 500 });
    log.append(document.createElement("p"));
    await frame();
    await frame();
    expect(log.dataset.cue).toBe("y");
    stop();
  });
});
