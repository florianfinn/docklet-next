// Source: docklet-next/issues/436
// Revision: b11b3e70e671dd3c862b966b1d279251842c7319
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original native sort button tooltip and interaction regressions.

import { act, render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { useState } from "react";
import { describe, expect, it, vi } from "vitest";

import type { SortState } from "@/shared/lib/table-sort";
import { BareTable, TableBody, TableCell, TableHeader, TableRow } from "@/shared/ui/table";
import { TooltipProvider } from "@/shared/ui/tooltip";
import { SortableHead, SortButton } from "./sortable-head";

const HELP = "Severity score from the scan, 0–10.";

function Harness() {
  const [sort, setSort] = useState<SortState<"score">>({ key: "score", dir: "asc" });
  return <TooltipProvider><BareTable>
    <TableHeader><TableRow><SortableHead sortKey="score" sort={sort} onSort={(key) => setSort({ key, dir: sort.dir === "asc" ? "desc" : "asc" })} label="Sort by score" help={HELP} filtered filteredLabel="Filtered">Score</SortableHead></TableRow></TableHeader>
    <TableBody>{[2, 8].sort((a, b) => sort.dir === "asc" ? a - b : b - a).map((score) => <TableRow key={score}><TableCell>{score}</TableCell></TableRow>)}</TableBody>
  </BareTable></TooltipProvider>;
}

describe("native sort button help", () => {
  it("opens on keyboard focus with an existing description target and keeps sorting and markers", async () => {
    const { container } = render(<Harness />);
    const user = userEvent.setup();
    const button = screen.getByRole("button", { name: "Sort by score (Filtered)" });
    await user.tab();
    expect(document.activeElement === button).toBe(true);
    const tooltip = screen.queryByRole("tooltip");
    expect(tooltip !== null).toBe(true);
    const description = document.getElementById(button.getAttribute("aria-describedby") ?? "");
    expect(description !== null && description === tooltip).toBe(true);
    expect(description?.textContent).toBe(HELP);
    expect(button.tagName).toBe("BUTTON");
    expect(container.querySelector("button button") !== null).toBe(false);
    expect(button.closest("th")?.getAttribute("aria-sort")).toBe("ascending");
    expect([...container.querySelectorAll("tbody td")].map((c) => c.textContent)).toEqual(["2", "8"]);
    expect(button.querySelector("svg")?.classList.contains("lucide-arrow-up")).toBe(true);
    await user.click(button);
    expect(button.closest("th")?.getAttribute("aria-sort")).toBe("descending");
    expect([...container.querySelectorAll("tbody td")].map((c) => c.textContent)).toEqual(["8", "2"]);
    expect(button.querySelector("svg")?.classList.contains("lucide-arrow-down")).toBe(true);
    expect(button.querySelector('[data-slot="filter-marker"]') !== null).toBe(true);
    await user.keyboard("{Enter}");
    expect(button.closest("th")?.getAttribute("aria-sort")).toBe("ascending");
  });

  it("opens on hover and closes on Escape", async () => {
    render(<Harness />);
    const user = userEvent.setup();
    expect(screen.queryByRole("tooltip") !== null).toBe(false);
    await user.hover(screen.getByRole("button", { name: "Sort by score (Filtered)" }));
    expect(screen.queryByRole("tooltip")?.textContent).toBe(HELP);
    await user.keyboard("{Escape}");
    expect(screen.queryByRole("tooltip") !== null).toBe(false);
  });

  it("keeps the provider-free sort button unchanged when help is absent", () => {
    const onSort = vi.fn();
    render(<SortButton sortKey="name" sort={{ key: "name", dir: "asc" }} onSort={onSort} label="Sort by name">Name</SortButton>);
    const button = screen.getByRole("button", { name: "Sort by name" });
    act(() => { button.focus(); button.click(); });
    expect(button.hasAttribute("aria-describedby")).toBe(false);
    expect(onSort).toHaveBeenCalledWith("name");
  });
});
