// Source: docklet-next/issues/266
// Revision: 5f56262875af9a8ec0fe9007cb889bd3a513085d
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original per-view, per-tab and per-locale responsive security regressions.

import { screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, describe, expect, it, vi } from "vitest";

import { HOSTS } from "@/data/demo/fixtures/hosts";
import { catalogs } from "@/app/i18n/catalog";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { Finding, ScannedImage } from "@/domain/model";
import type { FitView } from "@/shared/lib/fit-view";
import { renderWithApp } from "@/test/render";

import { FindingsTab, ImagesTab } from "./SecurityTabs";
import { FINDING_GROUPS, IMAGE_GROUPS, securityColumnGroups } from "./security-layout";

const fit = vi.hoisted(() => ({ view: "table" as FitView }));
vi.mock("@/shared/hooks/use-fit-view", () => ({ useFitView: () => ({ view: fit.view, ref: { current: null } }) }));

const findings: Finding[] = [
  { id: "CVE-2026-1001", severity: "high", pkg: "package-" + "long".repeat(20), installed: "1.0.0-" + "build".repeat(20), fixedIn: "2.0.0-" + "fixed".repeat(20), image: "registry.example.com/" + "image".repeat(20) + ":1.0.0", hostId: "example", cvss: 8.1, risk: 9.2, epss: 0.1, title: "Example finding" },
  { id: "CVE-2026-1002", severity: "low", pkg: "other", installed: "1.0.0", image: "registry.example.com/other:1.0.0", hostId: "example", cvss: 2.1, risk: 1.2, epss: 0.01, title: "Other finding" },
];
const images: ScannedImage[] = findings.map((f, i) => ({ id: `image-${i}`, ref: f.image, hostId: "example", usage: i ? "unused" : "running", counts: { critical: i ? 0 : 2, high: i ? 0 : 1, medium: 0, low: 0, unknown: 0 }, scannedAt: "2026-11-05T03:00:00Z", platform: "linux/amd64" }));
afterEach(() => { setAppLocale("de"); fit.view = "table"; });

async function choose(control: HTMLElement, option: string) {
  control.focus();
  await userEvent.keyboard("{Enter}");
  await userEvent.click(await screen.findByRole("option", { name: option }));
}

for (const locale of ["en-US", "de"] as const) {
  const messages = catalogs[locale];
  const t = messages.security;
  for (const view of ["table", "condensed", "cards"] as const) {
    describe(`${locale} ${view}`, () => {
      it("retains every finding field and supports a filter, reset and sorting", async () => {
        setAppLocale(locale); fit.view = view;
        const { container } = renderWithApp(<FindingsTab findings={findings} hosts={new Map([["example", { ...HOSTS[0]!, id: "example", name: "Example host" }]])} highlight={findings[0]!.id} />);
        const columns = ["id", "severity", "pkg", "installed", "fixed", "image", "host", "cvss", "risk", "epss"] as const;
        expect(securityColumnGroups(view, columns, FINDING_GROUPS).flat()).toEqual([...columns]);
        const rows = () => [...container.querySelectorAll('[data-security-row]')];
        expect(rows().length).toBe(2);
        const row = rows()[0]!;
        expect(row.getAttribute("data-state")).toBe("selected");
        expect(row.textContent?.includes(findings[0]!.image)).toBe(true);
        expect(row.textContent?.includes(findings[0]!.pkg)).toBe(true);
        expect(row.textContent?.includes(findings[0]!.installed)).toBe(true);
        expect(row.textContent?.includes(findings[0]!.fixedIn!)).toBe(true);
        expect(row.textContent?.includes("10.00%")).toBe(true);
        if (view !== "table") {
          for (const column of columns) {
            const field = row.querySelector(`[data-field="${column}"]`);
            expect(field !== null).toBe(true);
            expect(field?.querySelector("dt")?.textContent).toBe(t.columns[column]);
            expect((field?.querySelector("dd")?.textContent?.length ?? 0) > 0).toBe(true);
          }
        }
        const packageFilter = screen.getByRole("searchbox", { name: messages.common.columnFilters.label.replace("{column}", t.columns.pkg) });
        await userEvent.type(packageFilter, "other");
        expect(rows().length).toBe(1);
        expect(rows()[0]!.textContent?.includes("CVE-2026-1002")).toBe(true);
        await userEvent.click(screen.getByRole("button", { name: messages.common.columnFilters.reset }));
        expect(rows().length).toBe(2);
        if (view === "table") {
          await userEvent.click(screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.columns.pkg) }));
        } else {
          await choose(screen.getByRole("combobox", { name: messages.common.overview.sortKey }), t.columns.pkg);
        }
        expect(rows()[0]!.textContent?.includes("CVE-2026-1002")).toBe(true);
        if (view !== "table") {
          await userEvent.click(screen.getByRole("button", { name: new RegExp(messages.common.overview.directionToggle) }));
          expect(rows()[0]!.textContent?.includes("CVE-2026-1001")).toBe(true);
        }
      });
      it("retains every image field and supports choice filters, reset and sorting", async () => {
        setAppLocale(locale); fit.view = view;
        const { container } = renderWithApp(<ImagesTab images={images} stacks={[]} hosts={new Map([["example", { ...HOSTS[0]!, id: "example", name: "Example host" }]])} />);
        const columns = ["image", "stack", "host", "usage", "critical", "high", "medium", "low", "unknown", "scanned"] as const;
        expect(securityColumnGroups(view, columns, IMAGE_GROUPS).flat()).toEqual([...columns]);
        const rows = () => [...container.querySelectorAll('[data-security-row]')];
        expect(rows().length).toBe(2);
        expect(rows()[0]!.textContent?.includes(images[0]!.ref)).toBe(true);
        expect(rows()[0]!.textContent?.includes("linux/amd64")).toBe(true);
        if (view !== "table") {
          for (const column of columns) {
            const field = rows()[0]!.querySelector(`[data-field="${column}"]`);
            expect(field !== null).toBe(true);
            expect(field?.querySelector("dt")?.textContent).toBe(column in t.columns ? t.columns[column as keyof typeof t.columns] : t.severities[column as keyof typeof t.severities]);
            expect((field?.querySelector("dd")?.textContent?.length ?? 0) > 0).toBe(true);
          }
        }
        await choose(screen.getByRole("combobox", { name: messages.common.columnFilters.label.replace("{column}", t.columns.usage) }), t.usage.unused);
        expect(rows().length).toBe(1);
        expect(rows()[0]!.textContent?.includes(images[1]!.ref)).toBe(true);
        await userEvent.click(screen.getByRole("button", { name: messages.common.columnFilters.reset }));
        expect(rows().length).toBe(2);
        if (view === "table") {
          await userEvent.click(screen.getByRole("button", { name: t.filters.sortBy.replace("{column}", t.severities.critical) }));
        } else {
          await userEvent.click(screen.getByRole("button", { name: new RegExp(messages.common.overview.directionToggle) }));
        }
        expect(rows()[0]!.textContent?.includes(images[1]!.ref)).toBe(true);
        expect(within(container).getAllByRole("combobox").length >= 7).toBe(true);
      });
    });
  }
}
