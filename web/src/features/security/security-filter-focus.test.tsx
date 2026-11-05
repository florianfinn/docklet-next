// Source: docklet-next/issues/266
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original compact filter focus guards with the real measured view policy.

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterAll, beforeAll, describe, expect, it } from "vitest";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { Finding, ScannedImage } from "@/domain/model";
import type { FitView } from "@/shared/lib/fit-view";
import { renderWithApp } from "@/test/render";
import { FindingsTab, ImagesTab } from "./SecurityTabs";

const findings: Finding[] = [
  { id: "CVE-2026-1001", severity: "high", pkg: "package-long", installed: "1.0.0", fixedIn: "2.0.0", image: "registry.example.com/image:1.0.0", hostId: "example", cvss: 8.1, risk: 9.2, epss: 0.1, title: "Example finding" },
  { id: "CVE-2026-1002", severity: "low", pkg: "other", installed: "1.0.0", image: "registry.example.com/other:1.0.0", hostId: "example", cvss: 2.1, risk: 1.2, epss: 0.01, title: "Other finding" },
] as Finding[];
const images: ScannedImage[] = findings.map((f, i) => ({ id: `example~image-${i}`, ref: f.image, hostId: "example", usage: i ? "unused" : "running", status: "completed", counts: { critical: 0, high: 1, medium: 0, low: 0, unknown: 0 }, scannedAt: "2026-11-05T03:00:00Z", platform: "linux/amd64" }));
const hosts = new Map([["example", { ...HOSTS[0]!, id: "example", name: "Example host" }]]);

// Stubbed layout: the narrowest view in `fits` is the first one that fits the 330 px container.
let fits: FitView = "cards";
const restore: (() => void)[] = [];
beforeAll(() => {
  const rect = HTMLElement.prototype.getBoundingClientRect;
  HTMLElement.prototype.getBoundingClientRect = function () {
    const view = this.closest("[data-view]")?.getAttribute("data-view");
    const w = this.tagName === "UL" ? 300 : this.tagName === "TABLE" ? (view === "condensed" && fits === "condensed" ? 300 : 1000) : 0;
    return { width: w, height: 10, x: 0, y: 0, top: 0, left: 0, right: w, bottom: 10, toJSON() {} } as DOMRect;
  };
  const desc = Object.getOwnPropertyDescriptor(HTMLElement.prototype, "clientWidth");
  Object.defineProperty(HTMLElement.prototype, "clientWidth", { configurable: true, get() { return 330; } });
  restore.push(() => { HTMLElement.prototype.getBoundingClientRect = rect; if (desc) Object.defineProperty(HTMLElement.prototype, "clientWidth", desc); });
});
afterAll(() => restore.forEach((r) => r()));

const view = () => document.querySelector('[data-slot="security-table"]')!.getAttribute("data-view");
const filters = () => document.querySelector('[data-slot="security-filters"]')!;

describe.each(["condensed", "cards"] as const)("compact filters stay usable in %s", (layout) => {
  it.each([
    ["findings", "pkg", "ot", "severity"],
    ["images", "image", "ot", "usage"],
  ] as const)("%s tab keeps text and choice filter focus", async (tab, textColumn, typed, choiceColumn) => {
    fits = layout; setAppLocale("en-US");
    const security = catalogs["en-US"].security;
    renderWithApp(tab === "findings" ? <FindingsTab findings={findings} hosts={hosts} highlight={null} /> : <ImagesTab images={images} stacks={[]} hosts={hosts} />);
    expect(view()).toBe(layout);
    const label = (column: string) => catalogs["en-US"].common.columnFilters.label.replace("{column}", security.columns[column as keyof typeof security.columns]);
    const input = screen.getByRole("searchbox", { name: label(textColumn) }) as HTMLInputElement;
    await userEvent.click(input);
    await userEvent.keyboard(typed);
    expect(view()).toBe(layout);
    expect(screen.getByRole("searchbox", { name: label(textColumn) }) === input).toBe(true);
    expect(input.value).toBe(typed);
    expect(document.activeElement === input).toBe(true);
    await userEvent.clear(input);
    const trigger = filters().querySelector(`[aria-label="${label(choiceColumn)}"]`) as HTMLElement;
    trigger.focus();
    await userEvent.keyboard("{Enter}");
    await userEvent.keyboard("{ArrowDown}{Enter}");
    expect(view()).toBe(layout);
    expect(filters().querySelector(`[aria-label="${label(choiceColumn)}"]`) === trigger).toBe(true);
    expect(trigger.textContent === catalogs["en-US"].common.columnFilters.all).toBe(false);
  });
});
