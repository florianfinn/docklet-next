// Source: docklet-next/issues/585
// Revision: 34fb271fc0cea26ae89b231dc8f4dbfbb3531376
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original finding scale, occurrence, navigation, filter and measurement guards.

import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { advisoryKey } from "@/data/vulnerability-advisory";
import { occurrenceSelectionKey } from "./advisory-selection";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { Finding } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { FindingsTab } from "./SecurityTabs";

const findings: Finding[] = Array.from({ length: 10_000 }, (_, i) => ({
  id: i === 4998 || i === 4999 ? "CVE-2026-12345" : `CVE-2026-${String(i + 20000)}`,
  pkg: `pkg-${String(i).padStart(5, "0")}`, installed: "1.0", image: `example/app-${i % 2}:1.0`,
  hostId: "example", cvss: 8.123456, severity: "high", title: "Example finding",
}));
const hosts = new Map();
const key = (i: number) => { const f = findings[i]!; return JSON.stringify([f.hostId, f.image, f.id, f.pkg, f.installed, "", ""]); };
const rows = () => [...document.querySelectorAll<HTMLElement>("[data-security-row]")];
const selected = () => rows().filter((row) => row.dataset.state === "selected");
const page = () => screen.getByRole("navigation", { name: "Pagination" }).textContent;
const bounded = () => expect(rows().length).toBeLessThanOrEqual(100);
const props = { findings, hosts, highlight: null };

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { vi.restoreAllMocks(); setAppLocale("de"); });

describe("bounded finding pages", () => {
  it("mounts 100 of 10,000 findings on the first render", () => {
    renderWithApp(<FindingsTab {...props} />);
    expect(rows().length).toBe(100);
    expect(page()).toContain("Page 1 of 100");
    expect(screen.getByText("1–100 of 10,000") !== null).toBe(true);
  });

  it("jumps to a late occurrence without prefix expansion and highlights only it", () => {
    renderWithApp(<FindingsTab {...props} highlight={key(4999)} />);
    bounded();
    expect(page()).toContain("Page 50 of 100");
    expect(selected().length).toBe(1);
    expect(selected()[0]?.textContent?.includes("pkg-04999")).toBe(true);
  });

  it("resolves a legacy advisory to its first occurrence, highlighting one shared advisory row", () => {
    renderWithApp(<FindingsTab {...props} highlight="CVE-2026-12345" />);
    bounded();
    expect(selected().length).toBe(1);
    expect(selected()[0]?.textContent?.includes("pkg-04998")).toBe(true);
  });

  it("pages through all pages and back with correct membership and a fixed ceiling", async () => {
    // 1,000 findings keep the full forward/backward walk fast under suite load; the 10,000 cases above cover scale.
    renderWithApp(<FindingsTab {...props} findings={findings.slice(0, 1_000)} />);
    const next = screen.getByRole("button", { name: "Next" });
    const previous = screen.getByRole("button", { name: "Previous" });
    for (let p = 1; p <= 10; p++) {
      bounded();
      expect(rows().length).toBe(100);
      expect(rows()[0]?.textContent?.includes(`pkg-${String((p - 1) * 100).padStart(5, "0")}`)).toBe(true);
      if (p < 10) await act(async () => { fireEvent.click(next); });
    }
    expect((next as HTMLButtonElement).disabled).toBe(true);
    for (let p = 9; p >= 1; p--) { await act(async () => { fireEvent.click(previous); }); bounded(); }
    expect(page()).toContain("Page 1 of 10");
    const input = screen.getByRole("searchbox", { name: "Filter column Package" });
    fireEvent.change(input, { target: { value: "pkg-000" } });
    expect(rows().length).toBe(100);
    expect(rows().every((r) => r.textContent?.includes("pkg-000"))).toBe(true);
    expect(screen.getByText("1–100 of 100 · 1,000 in total") !== null).toBe(true);
  });

  it("keeps a target across sort/filter changes, resets an excluded target and restores it", () => {
    renderWithApp(<FindingsTab {...props} highlight={key(4999)} />);
    fireEvent.click(screen.getByRole("button", { name: "Sort by Package" }));
    expect(page()).toContain("Page 50 of 100");
    fireEvent.click(screen.getByRole("button", { name: "Sort by Package" }));
    expect(page()).toContain("Page 51 of 100");
    const input = screen.getByRole("searchbox", { name: "Filter column Package" });
    fireEvent.change(input, { target: { value: "pkg-04" } });
    expect(page()).toContain("Page 1 of 10");
    expect(selected().length).toBe(1);
    fireEvent.change(input, { target: { value: "pkg-00" } });
    expect(page()).toContain("Page 1 of 10");
    expect(screen.getByRole("status").textContent).toBe("Finding hidden by filters");
    expect(selected().length).toBe(0);
    bounded();
    fireEvent.change(input, { target: { value: "" } });
    expect(page()).toContain("Page 51 of 100");
    expect(selected().length).toBe(1);
  });

  it("keeps identical occurrence rows distinct and selects only their first row", () => {
    const errors = vi.spyOn(console, "error").mockImplementation(() => {});
    renderWithApp(<FindingsTab {...props} findings={[findings[4999]!, { ...findings[4999]! }]} highlight={key(4999)} />);
    expect(rows().length).toBe(2);
    expect(selected().length).toBe(1);
    expect(errors.mock.calls.some((args) => args.some((value) => String(value).includes("same key")))).toBe(false);
  });

  it("resets ordinary navigation on sorting/filtering and supports keyboard paging", async () => {
    renderWithApp(<FindingsTab {...props} />);
    const next = screen.getByRole("button", { name: "Next" });
    next.focus();
    await userEvent.keyboard("{Enter}");
    expect(page()).toContain("Page 2 of 100");
    fireEvent.click(screen.getByRole("button", { name: "Sort by Package" }));
    expect(page()).toContain("Page 1 of 100");
    fireEvent.click(next);
    fireEvent.change(screen.getByRole("searchbox", { name: "Filter column Package" }), { target: { value: "pkg-04" } });
    expect(page()).toContain("Page 1 of 10");
  });

  it("resolves a pending target after refresh and retains its occurrence when data moves", async () => {
    const view = renderWithApp(<FindingsTab {...props} findings={[]} highlight="CVE-2026-12345" />);
    await act(() => view.router.navigate("/", { state: "refresh" }));
    // Updating the route element models an uncapped read settling after a deep link.
    view.router.routes[0]!.element = <FindingsTab {...props} highlight="CVE-2026-12345" />;
    await act(() => view.router.revalidate());
    expect(selected().length).toBe(1);
    expect(page()).toContain("Page 50 of 100");
    bounded();
    view.router.routes[0]!.element = <FindingsTab {...props} findings={findings.toReversed()} highlight="CVE-2026-12345" />;
    await act(() => view.router.revalidate());
    expect(page()).toContain("Page 51 of 100");
    expect(selected()[0]?.textContent?.includes("pkg-04998")).toBe(true);
    bounded();
  });


  it("honours encoded URL advisory/occurrence links and retains the page after sheet dismissal", async () => {
    const view = renderWithApp(<FindingsTab {...props} />);
    const f = findings[4999]!;
    const occurrence = occurrenceSelectionKey({ host: f.hostId, imageId: "", package: f.pkg, installed: f.installed, class: "", type: "" });
    await act(() => view.router.navigate(`/?finding=${advisoryKey(f.id)}&occ=${occurrence}`));
    expect(page()).toContain("Page 50 of 100");
    expect(selected().length).toBe(1);
    expect(selected()[0]?.textContent?.includes("pkg-04999")).toBe(true);
    bounded();
    await act(() => view.router.navigate("/"));
    expect(page()).toContain("Page 50 of 100");
    expect(selected().length).toBe(1);
  });

  it("constructs cells and measures only the visible 100 rows in every responsive view", () => {
    const measured: number[] = [];
    vi.spyOn(HTMLElement.prototype, "getBoundingClientRect").mockImplementation(function (this: HTMLElement) {
      if (this.tagName === "TABLE" || this.tagName === "UL") {
        const count = this.querySelectorAll("[data-security-row]").length;
        measured.push(count);
        expect(count, "every layout measurement must see only the visible page").toBe(100);
      }
      return { width: this.tagName === "TABLE" ? 1000 : 300 } as DOMRect;
    });
    vi.spyOn(HTMLElement.prototype, "clientWidth", "get").mockReturnValue(330);
    const constructed = vi.spyOn(Number.prototype, "toFixed");
    const cellConstructions = () => constructed.mock.contexts.filter((value) => Number(value) === 8.123456).length;
    renderWithApp(<FindingsTab {...props} />);
    expect(cellConstructions()).toBe(100);
    expect(measured).toEqual([100, 100, 100]);
    expect(document.querySelector("[data-view]")?.getAttribute("data-view")).toBe("cards");
    bounded();
    constructed.mockClear(); measured.length = 0;
    fireEvent.click(screen.getByRole("button", { name: "Next" }));
    expect(cellConstructions()).toBe(100);
    expect(measured).toEqual([100]);
  });
});
