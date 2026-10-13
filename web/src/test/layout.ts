import { vi } from "vitest";

/** happy-dom has no layout: report a wide workspace frame so the aside column renders. */
export function mockWideWorkspace(width = 1600) {
  const original = HTMLElement.prototype.getBoundingClientRect;
  return vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
    return this.dataset.slot === "workspace-frame" ? new DOMRect(0, 0, width, 800) : original.call(this);
  });
}
