// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original pattern presentation regression evidence for M47.

import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
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

it("drops info rows when the level filter splits one shape by level", async () => {
  setAppLocale("en-US");
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const messages = [...Array(3).fill('level="info" msg="disk full"'), "[W] retry soon", "[W] retry soon", 'level="error" msg="disk full"'];
  const demo = createDemoAdapter();
  const adapter: typeof demo = { ...demo, readLogs: vi.fn(async (_resource, _options, sink) => {
    sink(messages.map((message, i) => ({ id: String(i), service: "web", ts: "2026-01-02T03:00:00Z", level: "info", message })));
    return { reason: "end" as const, lines: messages.length, bytes: 100 };
  }) };
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await act(async () => { await Promise.resolve(); });
  while (callbacks.length) act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  expect(screen.getAllByRole("img")).toHaveLength(3);
  await userEvent.setup().click(screen.getByRole("radio", { name: "Error + warning" }));
  expect(screen.getAllByRole("img")).toHaveLength(2);
  expect(screen.queryByText("Info") === null).toBe(true);
});
