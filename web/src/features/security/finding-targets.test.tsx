// Source: docklet-next/issues/585
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original target, occurrence and anchoring proofs for paged findings (M37 #585 R1).

import { act, fireEvent, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useEffect, useState } from "react";
import { afterEach, beforeEach, expect, it } from "vitest";

import { setAppLocale } from "@/app/i18n/locale-store";
import { advisoryKey } from "@/data/vulnerability-advisory";
import type { Finding } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { occurrenceSelectionKey } from "./advisory-selection";
import { FindingsTab } from "./SecurityTabs";

const base = (i: number): Finding => ({ id: `CVE-2026-${20000 + i}`, pkg: `pkg-${String(i).padStart(5, "0")}`, installed: "1.0", image: "example/app:1.0", imageId: "sha256:aa", hostId: "example", cvss: 5, severity: "high", title: "t" });
const findings = Array.from({ length: 1_000 }, (_, i) => base(i));
const rows = () => [...document.querySelectorAll<HTMLElement>("[data-security-row]")];
const selected = () => rows().filter((r) => r.dataset.state === "selected");
const page = () => screen.getByRole("navigation", { name: "Pagination" }).textContent ?? "";
const hosts = new Map();

beforeEach(() => setAppLocale("en-US"));
afterEach(() => setAppLocale("de"));

/** Lets a test replace the findings like a background refresh does, without remounting the tab. */
function Live({ initial, highlight }: { initial: Finding[]; highlight: string | null }) {
  const [data, setData] = useState(initial);
  useEffect(() => { live.set = setData; }, []);
  return <FindingsTab findings={data} hosts={hosts} highlight={highlight} />;
}
const live = { set: (_: Finding[]) => {} };

it("an occ link selects the occurrence that differs only in class and type", async () => {
  const a = { ...base(1), class: "os-pkgs", type: "debian", fixedIn: "A-FIX" } as Finding;
  const b = { ...base(1), class: "lang-pkgs", type: "gobinary", fixedIn: "B-FIX" } as Finding;
  const view = renderWithApp(<FindingsTab findings={[a, b]} hosts={hosts} highlight={null} />);
  const occ = occurrenceSelectionKey({ host: "example", imageId: "sha256:aa", package: b.pkg, installed: "1.0", class: "lang-pkgs", type: "gobinary" });
  await act(() => view.router.navigate(`/?finding=${advisoryKey(b.id)}&occ=${occ}`));
  expect(selected().length).toBe(1);
  expect(selected()[0]?.textContent?.includes("B-FIX")).toBe(true);
});

it("a target that is absent or not loaded yet is never reported as hidden by filters", async () => {
  renderWithApp(<Live initial={[]} highlight="CVE-1999-0001" />);
  expect(screen.queryByText("Finding hidden by filters") === null).toBe(true);
  act(() => live.set(findings));
  expect(rows().length).toBe(100);
  expect(screen.queryByText("Finding hidden by filters") === null).toBe(true);
});

it("a target that exists but is filtered out is reported as hidden by filters", () => {
  renderWithApp(<FindingsTab findings={findings} hosts={hosts} highlight={findings[150]!.id} />);
  expect(page().includes("Page 2 of 10")).toBe(true);
  fireEvent.change(screen.getByRole("searchbox", { name: "Filter column Package" }), { target: { value: "pkg-0000" } });
  expect(screen.getByRole("status").textContent).toBe("Finding hidden by filters");
});

it("a background refresh that moves the target does not pull a manually paging user back", async () => {
  renderWithApp(<Live initial={findings} highlight={findings[150]!.id} />);
  expect(page().includes("Page 2 of 10")).toBe(true);
  const next = screen.getByRole("button", { name: "Next" });
  for (let i = 0; i < 5; i++) await act(async () => { fireEvent.click(next); });
  expect(page().includes("Page 7 of 10")).toBe(true);
  // A newly scanned finding sorts before the target (higher risk); late data before paging still follows the target.
  act(() => live.set([{ ...base(5000), cvss: 9.9, risk: 9.9 }, ...findings]));
  expect(page().includes("Page 7 of 11")).toBe(true);
});

it("follows a target that data moves while the user has not paged", async () => {
  const target = findings[199]!;
  renderWithApp(<Live initial={findings} highlight={target.id} />);
  expect(page().includes("Page 2 of 10")).toBe(true);
  act(() => live.set([{ ...base(5000), cvss: 9.9, risk: 9.9 }, ...findings]));
  expect(page().includes("Page 3 of 11")).toBe(true);
  expect(selected()[0]?.textContent?.includes(target.id)).toBe(true);
});

it("keeps keyboard focus on the pager when a button becomes disabled at an end", async () => {
  renderWithApp(<FindingsTab findings={findings.slice(0, 250)} hosts={hosts} highlight={null} />);
  const next = screen.getByRole("button", { name: "Next" });
  next.focus();
  await userEvent.keyboard("{Enter}");
  await userEvent.keyboard("{Enter}");
  expect(page().includes("Page 3 of 3")).toBe(true);
  expect(document.activeElement === screen.getByRole("button", { name: "Previous" })).toBe(true);
});

it("shows no pager for a single page", () => {
  renderWithApp(<FindingsTab findings={findings.slice(0, 40)} hosts={hosts} highlight={null} />);
  expect(screen.queryByRole("navigation", { name: "Pagination" }) === null).toBe(true);
  expect(rows().length).toBe(40);
});
