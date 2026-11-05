// Source: docklet-next/issues/280
// Revision: 5a4ecc67a7712349cb869574429d09f09a96e895
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original guard that the measured image overview keeps per-row findings chips.

import { screen, waitFor } from "@testing-library/react";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { catalogs } from "@/app/i18n/catalog";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";
import type { FitView } from "@/shared/lib/fit-view";
import { renderWithApp } from "@/test/render";

// Stubbed layout: long references overflow the full table; `fits` names the widest view that fits.
let fits: FitView = "condensed";
const restore: (() => void)[] = [];
beforeAll(() => {
  const rect = HTMLElement.prototype.getBoundingClientRect;
  HTMLElement.prototype.getBoundingClientRect = function () {
    const view = this.getAttribute("data-view");
    const w = view === "cards" ? 300 : view === "condensed" ? (fits === "condensed" ? 300 : 1500) : view === "table" ? 1500 : 0;
    return { width: w, height: 10, x: 0, y: 0, top: 0, left: 0, right: w, bottom: 10, toJSON() {} } as DOMRect;
  };
  const desc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth");
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return 330; } });
  restore.push(() => { HTMLElement.prototype.getBoundingClientRect = rect; if (desc) Object.defineProperty(HTMLElement.prototype, "clientWidth", desc); });
});
afterAll(() => restore.forEach((r) => r()));

describe.each(["condensed", "cards"] as const)("measured image overview in %s", (layout) => {
  it("shows every scanned image's findings chip with its counts and link", async () => {
    fits = layout;
    const { default: ImagesPage } = await import("./ImagesPage");
    const f = securityFixture();
    const scanned = (await f.adapter.listImages()).filter((i) => i.id.endsWith("~sha256:user"));
    expect(scanned.length > 1).toBe(true);
    renderWithApp(<ImagesPage />, f.adapter);
    const severities = catalogs.de.security.severities;
    // Fixture summary: 2 critical, 1 high, 0 medium, 0 low, 3 unknown.
    const label = catalogs.de.images.securityLabel.replace("{all}", `2 ${severities.critical}, 1 ${severities.high}, 0 ${severities.medium}, 0 ${severities.low}, 3 ${severities.unknown}`);
    await waitFor(() => expect(screen.queryAllByRole("link", { name: label })).toHaveLength(scanned.length));
    expect(document.querySelector('[data-slot="overview-fit"]')!.getAttribute("data-view")).toBe(layout);
    for (const chip of screen.getAllByRole("link", { name: label })) {
      expect(chip.getAttribute("href")).toBe("/security?tab=images");
      expect(chip.textContent).toBe(`2 ${severities.critical}`);
    }
  });
});
