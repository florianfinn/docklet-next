// Source: docklet-next/issues/436
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original bilingual security header and responsive help regressions.

import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { Finding, ScannedImage } from "@/domain/model";
import type { FitView } from "@/shared/lib/fit-view";
import { renderWithApp } from "@/test/render";
import { FindingsTab, ImagesTab } from "./SecurityTabs";

const fit = vi.hoisted(() => ({ view: "table" as FitView }));
vi.mock("@/shared/hooks/use-fit-view", () => ({ useFitView: () => ({ view: fit.view, ref: { current: null } }) }));

const findings: Finding[] = [
  { id: "CVE-2026-1001", severity: "high", pkg: "first", installed: "1.0", fixedIn: "2.0", image: "example/app:1.0", hostId: "example", cvss: 8, risk: 9, epss: 0.1, title: "First finding" },
  { id: "CVE-2026-1002", severity: "low", pkg: "second", installed: "1.0", image: "example/app:1.0", hostId: "example", cvss: 2, risk: 1, epss: 0.01, title: "Second finding" },
];
const images: ScannedImage[] = [{ id: "example-image", ref: "example/app:1.0", hostId: "example", usage: "running", counts: { critical: 1, high: 2, medium: 3, low: 4, unknown: 5 }, scannedAt: "2026-11-09T00:00:00Z" }];
afterEach(() => { setAppLocale("de"); fit.view = "table"; });

function expectHelp(button: HTMLElement, expected: string) {
  act(() => button.focus());
  expect(document.activeElement === button).toBe(true);
  const tooltip = screen.queryByRole("tooltip");
  expect(tooltip !== null).toBe(true);
  const description = document.getElementById(button.getAttribute("aria-describedby") ?? "");
  expect(description !== null && description === tooltip).toBe(true);
  expect(description?.textContent).toBe(expected);
  act(() => button.blur());
}

for (const locale of ["en-US", "de"] as const) {
  const t = catalogs[locale].security;
  const about = (column: string) => t.headerHelp.label.replace("{column}", column);
  describe(`${locale} security help`, () => {
    it("explains finding headers on the actual sort buttons and preserves row order changes", async () => {
      setAppLocale(locale);
      const { container } = renderWithApp(<FindingsTab findings={findings} hosts={new Map()} highlight={null} />);
      for (const field of ["cvss", "risk", "epss", "installed", "fixed"] as const) {
        expectHelp(screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.columns[field]) }), t.headerHelp[field]);
      }
      const button = screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.columns.risk) });
      const firstId = () => container.querySelector("tbody tr td")?.textContent;
      expect(firstId()).toBe(findings[0]!.id);
      await userEvent.hover(button);
      expect(screen.queryByRole("tooltip")?.textContent).toBe(t.headerHelp.risk);
      await userEvent.click(button);
      expect(firstId()).toBe(findings[1]!.id);
      expect(button.closest("th")?.getAttribute("aria-sort")).toBe("ascending");
      expect(button.querySelector("svg")?.classList.contains("lucide-arrow-up")).toBe(true);
      expect(container.querySelector("button button") !== null).toBe(false);
    });

    it("explains usage and every abbreviated severity count", () => {
      setAppLocale(locale);
      renderWithApp(<ImagesTab images={images} stacks={[]} hosts={new Map()} />);
      expectHelp(screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.columns.usage) }), t.headerHelp.usage);
      for (const severity of ["critical", "high", "medium", "low", "unknown"] as const) {
        const button = screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.severities[severity]) });
        expect(button.querySelector("abbr")?.textContent).toBe(t.severities[severity].slice(0, 1));
        expectHelp(button, t.headerHelp.count.replace("{severity}", t.severities[severity]));
      }
    });

    for (const view of ["condensed", "cards"] as const) {
      it(`provides one keyboard reachable help icon per finding field in ${view}`, async () => {
        setAppLocale(locale); fit.view = view;
        renderWithApp(<FindingsTab findings={findings.slice(0, 1)} hosts={new Map()} highlight={null} />);
        const scope = view === "condensed" ? screen.getAllByRole("rowgroup")[0]! : screen.getByRole("list", { name: t.tabs.findings });
        for (const field of ["cvss", "risk", "epss", "installed", "fixed"] as const) {
          const button = within(scope).getByRole("button", { name: about(t.columns[field]) });
          // Reach it with a real Tab gesture from its preceding focusable control.
          act(() => button.focus());
          await userEvent.tab({ shift: true });
          await userEvent.tab();
          expect(document.activeElement === button).toBe(true);
          expectHelp(button, t.headerHelp[field]);
        }
        expect(document.querySelector("button button") !== null).toBe(false);
      });

      it(`keeps usage and each severity help accessible in ${view}`, () => {
        setAppLocale(locale); fit.view = view;
        renderWithApp(<ImagesTab images={images} stacks={[]} hosts={new Map()} />);
        expectHelp(screen.getByRole("button", { name: about(t.columns.usage) }), t.headerHelp.usage);
        for (const severity of ["critical", "high", "medium", "low", "unknown"] as const) {
          expectHelp(screen.getByRole("button", { name: about(t.severities[severity]) }), t.headerHelp.count.replace("{severity}", t.severities[severity]));
        }
      });
    }
  });
}
