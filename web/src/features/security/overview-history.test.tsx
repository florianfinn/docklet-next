// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: 6e397e20b140d220d9306dc97d71f52731964f26
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original history chart label regression for issue #262.

import type { ReactNode } from "react";
import { screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import { renderWithApp } from "@/test/render";
import { securityFixture } from "@/data/http/vulnerabilities.test-support";
import { OverviewTab } from "./SecurityTabs";

// Inspect the labels passed to the chart renderer without relying on SVG layout in happy-dom.
vi.mock("@/shared/ui/chart", async (original) => ({ ...await original<typeof import("@/shared/ui/chart")>(), ChartContainer: ({ children }: { children: ReactNode }) => <div>{children}</div> }));
vi.mock("recharts", async (original) => ({ ...await original<typeof import("recharts")>(), BarChart: ({ data }: { data: { when: string }[] }) => <div data-testid="chart-labels">{data.map((r) => r.when).join("; ")}</div> }));

it("resolves host names in both history chart labels and the history list", async () => {
  const f = securityFixture(); f.state.hostNames.first = "Worker Alpha"; f.state.hostNames.second = "Worker Beta";
  const data = await f.adapter.getSecurity(), hosts = await f.adapter.listHosts();
  renderWithApp(<OverviewTab {...data} stacks={[]} hosts={new Map(hosts.map((h) => [h.id, h]))} animate={false} onFocus={vi.fn()} />, f.adapter);
  const labels = screen.getByTestId("chart-labels").textContent!;
  expect(labels).toContain("Worker Alpha"); expect(labels).toContain("Worker Beta");
  expect(labels).not.toContain("first"); expect(labels).not.toContain("second");
  expect(screen.getByRole("list", { name: "Befunde je Scan" }).textContent).toContain("Worker Alpha");
});
