// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original regression evidence for pattern bin reads, custom ranges and bin hints in M47.

import { act, fireEvent, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { LogReadOptions } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import type { LogLine } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { formatLogSpan } from "./log-time";
import { LogDensity } from "./LogInsights";
import { PatternMode } from "./PatternMode";

afterEach(() => { setAppLocale("de"); vi.unstubAllGlobals(); });
const T0 = Date.parse("2026-01-02T03:00:00Z");
const line = (i: number, ms: number, message: string): LogLine => ({ id: String(i), service: "web", ts: new Date(ms).toISOString(), level: "info", message });
const PATTERN_LINES = [...Array.from({ length: 30 }, (_, i) => line(i, T0 + i * 1000, "GET /a 200")), line(99, T0 + 15_000, "ERROR boom")];

function setup(slice: LogLine[] = []) {
  setAppLocale("en-US");
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const calls: LogReadOptions[] = [];
  const demo = createDemoAdapter();
  const adapter: typeof demo = { ...demo, readLogs: vi.fn(async (_ref, options, sink) => {
    calls.push(options);
    const lines = options.tail === "all" ? slice : PATTERN_LINES;
    sink(lines);
    return { reason: "end" as const, lines: lines.length, bytes: 100 };
  }) };
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  const drain = async () => {
    await act(async () => { await Promise.resolve(); });
    while (callbacks.length) act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  };
  return { calls, drain };
}
const strips = () => screen.getAllByRole("group", { name: "Distribution across 30 time bins in the covered span" });

it("reads exactly the tapped bin and marks the pattern's lines in it", async () => {
  const { calls, drain } = setup([line(1, T0, "GET /a 200"), line(2, T0 + 100, "worker tick")]);
  await drain();
  const first = within(strips()[0]!).getAllByRole("button")[0]!;
  await userEvent.setup().click(first);
  await act(async () => { await Promise.resolve(); });
  const read = calls.at(-1)!;
  expect(read.tail).toBe("all");
  expect(read.since).toBe(new Date(T0).toISOString());
  expect(read.until).toBe(new Date(T0 + 29_000 / 30).toISOString());
  const region = screen.getByRole("region", { name: `Lines ${formatLogSpan(T0, T0 + 29_000 / 30)}` });
  expect(within(region).getByText("GET /a 200").closest("li")?.hasAttribute("data-match")).toBe(true);
  expect(within(region).getByText("worker tick").closest("li")?.hasAttribute("data-match")).toBe(false);
});

it("shows the covered span once, not again on each pattern", async () => {
  const { drain } = setup();
  await drain();
  expect(screen.getAllByText(formatLogSpan(T0, T0 + 29_000))).toHaveLength(1);
});

it("reads a chosen custom range with both bounds", async () => {
  const { calls, drain } = setup();
  await drain();
  await userEvent.setup().click(screen.getByRole("radio", { name: "Range" }));
  fireEvent.change(screen.getByLabelText("From"), { target: { value: "2026-01-02T03:00" } });
  fireEvent.change(screen.getByLabelText("To"), { target: { value: "2026-01-02T04:00" } });
  await userEvent.setup().click(screen.getByRole("button", { name: "Apply" }));
  await drain();
  expect(calls.at(-1)).toMatchObject({ since: new Date("2026-01-02T03:00").toISOString(), until: new Date("2026-01-02T04:00").toISOString() });
});

it("shows a density bin's time and counts on hover", async () => {
  setAppLocale("en-US");
  const lines = [line(1, T0, "INFO ok"), { ...line(2, T0 + 30_000, "ERROR boom"), level: "error" as const }, line(3, T0 + 60_000, "INFO ok")];
  renderWithApp(<LogDensity lines={lines} onSelect={() => {}} />, createDemoAdapter());
  const bin = screen.getByRole("button");
  const label = bin.getAttribute("aria-label")!;
  expect(screen.queryByText(label)).toBeNull();
  await userEvent.setup().hover(bin);
  expect(screen.getByText(label).getAttribute("aria-hidden")).toBe("true");
});
