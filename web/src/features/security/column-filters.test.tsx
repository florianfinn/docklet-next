import { screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { simulationStore } from "@/data/demo/simulation";
import { renderRoutes } from "@/test/render";

import SecurityPage from "./SecurityPage";

beforeEach(() => simulationStore.set((s) => ({ ...s, latency: "instant", live: false })));
afterEach(() => {
  simulationStore.reset();
  localStorage.clear();
});

const table = (name: string) => screen.getByRole("table", { name });
const ids = () => within(table("Schwachstellen")).getAllByRole("row").slice(2).map((r) => r.querySelector("td")?.textContent);
const summary = () => document.querySelector('[data-slot="column-filter-summary"]')?.textContent;

async function choose(combobox: HTMLElement, option: string) {
  // Radix Select opens from the keyboard under happy-dom.
  combobox.focus();
  await userEvent.keyboard("{Enter}");
  await userEvent.click(await screen.findByRole("option", { name: option }));
}

describe("security column filters", () => {
  it("filter findings per column as a conjunction, count, show an empty state, reset and keep header sorting", async () => {
    renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=findings"]);
    await screen.findByRole("table", { name: "Schwachstellen" });
    const all = ids().length;
    expect(summary()).toBe(`${all} Befunde`);

    await choose(screen.getByRole("combobox", { name: "Spalte Schweregrad filtern" }), "Hoch");
    const high = ids();
    expect(high.length > 0 && high.length < all).toBe(true);
    await choose(screen.getByRole("combobox", { name: "Spalte Host filtern" }), "Boreal");
    expect(ids()).toEqual(["CVE-2026-09831", "CVE-2026-09720", "CVE-2026-09688"]);
    expect(summary()?.startsWith(`3 von ${all} Befunden · 2 Filter aktiv`)).toBe(true);

    await userEvent.type(screen.getByRole("searchbox", { name: "Spalte Paket filtern" }), "curl");
    expect(ids()).toEqual(["CVE-2026-09831"]);
    await userEvent.type(screen.getByRole("searchbox", { name: "Spalte Paket filtern" }), "-gibt-es-nicht");
    expect(screen.getByText("Keine Einträge für diese Spaltenfilter.") !== null).toBe(true);

    await userEvent.click(screen.getByRole("button", { name: "Filter zurücksetzen" }));
    expect([ids().length, summary()]).toEqual([all, `${all} Befunde`]);

    // Sorting still works on the header row above the filters (keyboard too).
    const pkg = screen.getByRole("button", { name: "Nach Paket sortieren" });
    pkg.focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(pkg.closest("th")?.getAttribute("aria-sort")).toBe("ascending"));
    const pkgs = within(table("Schwachstellen")).getAllByRole("row").slice(2).map((r) => r.querySelectorAll("td")[2]?.textContent ?? "");
    expect(pkgs).toEqual(pkgs.toSorted((a, b) => a.localeCompare(b, "de")));
  });

  it("filter the images table by usage and findings of one severity", async () => {
    renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=images"]);
    await screen.findByRole("table", { name: "Images" });
    const rows = () => within(table("Images")).getAllByRole("row").slice(2);
    const total = rows().length;
    await choose(screen.getByRole("combobox", { name: "Spalte Kritisch filtern" }), "mit Befunden");
    const critical = rows();
    expect(critical.length > 0 && critical.length < total && critical.every((r) => r.querySelectorAll("td")[4]?.textContent !== "0")).toBe(true);
    await choose(screen.getByRole("combobox", { name: "Spalte Nutzung filtern" }), "läuft");
    expect(rows().every((r) => r.querySelectorAll("td")[3]?.textContent === "läuft")).toBe(true);
  });

  it("mark filtered columns in the header, clear a text filter from its own button and keep the frame of the stacks tables", async () => {
    renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=findings"]);
    const findings = await screen.findByRole("table", { name: "Schwachstellen" });
    const head = (name: string) => within(findings).getAllByRole("columnheader").find((th) => th.textContent?.trim() === name)!;
    const marked = () => [...findings.querySelectorAll('th[data-filtered] [data-slot="filter-marker"]')].map((m) => m.closest("th")?.textContent?.trim());
    expect([!!findings.closest('[data-slot="security-table"]'), findings.className.includes("border-separate"), marked().length]).toEqual([true, true, 0]);

    const pkg = screen.getByRole("searchbox", { name: "Spalte Paket filtern" });
    await userEvent.type(pkg, "zlib");
    expect([ids(), marked(), head("Paket").getAttribute("data-filtered") === "true"]).toEqual([["CVE-2026-09402"], ["Paket"], true]);
    expect(screen.getByRole("button", { name: "Nach Paket sortieren (gefiltert)" }) !== null).toBe(true);
    await choose(screen.getByRole("combobox", { name: "Spalte Behoben in filtern" }), "mit Fix");
    expect(marked()).toEqual(["Paket", "Behoben in"]);

    // The clear button empties only its own column; focus stays usable and the count follows.
    await userEvent.click(screen.getByRole("button", { name: "Filter Paket leeren" }));
    expect([(pkg as HTMLInputElement).value, marked(), screen.queryByRole("button", { name: "Filter Paket leeren" }) === null]).toEqual(["", ["Behoben in"], true]);
    expect(summary()?.includes("1 Filter aktiv")).toBe(true);
    await userEvent.click(screen.getByRole("button", { name: "Filter zurücksetzen" }));
    expect([marked().length, document.querySelector('[data-slot="column-filter-summary"]')?.hasAttribute("data-active")]).toEqual([0, false]);
  });

  it("sort the images table by a severity count with the keyboard while a filter is set, and reset to every image", async () => {
    renderRoutes([{ path: "/security", element: <SecurityPage /> }], ["/security?tab=images"]);
    await screen.findByRole("table", { name: "Images" });
    const rows = () => within(table("Images")).getAllByRole("row").slice(2);
    const total = rows().length;
    await choose(screen.getByRole("combobox", { name: "Spalte Nutzung filtern" }), "läuft");
    const running = rows().length;
    const high = screen.getByRole("button", { name: "Nach Hoch sortieren" });
    high.focus();
    await userEvent.keyboard("{Enter}");
    await waitFor(() => expect(high.closest("th")?.getAttribute("aria-sort")).toBe("descending"));
    const counts = rows().map((r) => Number(r.querySelectorAll("td")[5]?.textContent));
    expect([counts.length, counts]).toEqual([running, counts.toSorted((a, b) => b - a)]);
    await userEvent.click(screen.getByRole("button", { name: "Filter zurücksetzen" }));
    expect([rows().length, high.closest("th")?.getAttribute("aria-sort")]).toEqual([total, "descending"]);
  });
});
