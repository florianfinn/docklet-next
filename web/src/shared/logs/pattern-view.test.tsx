// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original pattern presentation regression evidence for M47.

import { act, screen } from "@testing-library/react";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import { PatternMode } from "./PatternMode";

afterEach(() => { setAppLocale("de"); vi.unstubAllGlobals(); });
it("shows the poor-grouping notice and only the top 200 rows for the variable demo fixture", async () => {
  setAppLocale("en-US");
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const demo = createDemoAdapter({ logPatterns: "variable" });
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, demo);
  await act(async () => { await Promise.resolve(); });
  while (callbacks.length) act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  expect(screen.getByText("This log does not group well (5,000 patterns).")).toBeDefined();
  expect(screen.getAllByRole("img", { name: "Distribution across 30 time bins in the covered span" })).toHaveLength(200);
  expect(screen.getByText("Other shapes")).toBeDefined();
  expect(screen.queryByText("Reading and grouping…")).toBeNull();
});
