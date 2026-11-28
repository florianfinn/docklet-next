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
import { formatLogSpan, formatLogStamp } from "./log-time";
import { LogDensity } from "./LogInsights";
import { PatternMode } from "./PatternMode";
import { PatternSlice } from "./PatternSlice";
import { binBounds, binIndex, PatternFold } from "./patterns";
import { logFixture, settleLogs } from "@/data/http/logs.test-support";

afterEach(() => { setAppLocale("de"); vi.unstubAllGlobals(); vi.restoreAllMocks(); });
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
  expect(read.until).toBe("2026-01-02T03:00:00.966999999Z");
  const region = screen.getByRole("region", { name: `Lines ${formatLogSpan(T0, T0 + 966)}` });
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

it("bounds every bin by exactly the milliseconds the fold counted in it", () => {
  for (const [first, last] of [[T0, T0 + 29_000], [T0, T0 + 1_000], [T0, T0 + 7], [T0, T0]] as const) {
    const bounds = Array.from({ length: 30 }, (_, i) => binBounds(i, first, last));
    expect(bounds[0]!.start).toBe(first);
    expect(bounds[29]!.end).toBe(last + 1);
    for (let t = first; t <= last; t++) {
      const i = binIndex(t, first, last);
      expect(bounds[i]!.start <= t && t < bounds[i]!.end).toBe(true);
    }
    for (let i = 1; i < 30; i++) expect(bounds[i]!.start).toBe(bounds[i - 1]!.end);
  }
});

const head = { id: "one", ts: "2026-01-02T03:04:05.000Z", service: "web", level: "error" as const, message: "ERROR failure" };
it("lets the final bin's read reach the producer's nanoseconds of its last millisecond", async () => {
  setAppLocale("en-US");
  vi.spyOn(Date, "now").mockReturnValue(Date.parse(head.ts) + 60_000);
  const demo = createDemoAdapter(), fixture = logFixture();
  renderWithApp(<PatternMode kind="container" id="0~example" select={() => {}} />, { ...demo, readLogs: fixture.owner.readLogs });
  await act(settleLogs);
  act(() => { fixture.sockets[0]!.send({ seq: 1, service: "web", timestamp: head.ts, message: "ERROR failure" }); fixture.sockets[0]!.send({ seq: 2, service: "web", timestamp: "2026-01-02T03:04:35.000900000Z", message: "ERROR failure" }); fixture.sockets[0]!.disconnect(); fixture.flush(); });
  await act(settleLogs); act(fixture.flush);
  await userEvent.setup().click(within(strips()[0]!).getAllByRole("button").at(-1)!);
  await act(settleLogs);
  const query = new URL(fixture.urls.at(-1)!).searchParams;
  expect(query.get("until")).toBe("2026-01-02T03:04:35.000999999Z");
  expect(query.get("since")! > "2026-01-02T03:04:05.000Z").toBe(true);
});

it("states how many lines a capped bin read actually shows", async () => {
  setAppLocale("en-US");
  const fold = new PatternFold(); fold.add(head);
  const demo = createDemoAdapter(), fixture = logFixture();
  renderWithApp(<PatternSlice kind="container" id="0~example" pattern={fold.snapshot()[0]!} slice={{ start: Date.parse(head.ts), end: Date.parse(head.ts) + 1000 }} close={() => {}} />, { ...demo, readLogs: fixture.owner.readLogs });
  await act(settleLogs);
  act(() => { fixture.sockets[0]!.send({ seq: 1, timestamp: head.ts, message: "x".repeat(600 * 1024) }); fixture.sockets[0]!.send({ seq: 2, timestamp: head.ts, message: "x".repeat(600 * 1024) }); });
  await act(settleLogs);
  expect(screen.getByText("Showing the first line; this period has more.")).toBeDefined();
  expect(screen.getByRole("button", { name: "Load more lines" })).toBeDefined();
});

it("marks the continuation lines the fold counts in the pattern, per source", async () => {
  setAppLocale("en-US");
  const lines: LogLine[] = [
    { ...head, id: "lead", service: "api", message: "    at orphan (lead.js:1)" },
    head,
    { ...head, id: "api", service: "api", message: "INFO other" },
    { ...head, id: "two", message: "    at worker (entry.js:42)" },
    { ...head, id: "three", service: "api", message: "    at api (api.js:7)" },
  ];
  const fold = new PatternFold(); for (const l of lines.slice(1)) fold.add(l);
  const pattern = fold.snapshot().find((row) => row.key.startsWith("ERROR"))!;
  expect(pattern.count).toBe(2);
  const demo = createDemoAdapter();
  renderWithApp(<PatternSlice kind="container" id="example" pattern={pattern} slice={{ start: Date.parse(head.ts), end: Date.parse(head.ts) + 1000 }} close={() => {}} />, { ...demo, readLogs: async (_ref, _options, sink) => { sink(lines); return { reason: "end", lines: lines.length, bytes: 80 }; } });
  await act(settleLogs);
  const matched = (text: string) => screen.getByText(text).closest("li")!.hasAttribute("data-match");
  expect([matched("ERROR failure"), matched("at worker (entry.js:42)"), matched("at api (api.js:7)"), matched("at orphan (lead.js:1)")]).toEqual([true, true, false, false]);
});

it("scrolls the list to its first marked line", async () => {
  setAppLocale("en-US");
  const offsets = vi.spyOn(HTMLElement.prototype, "offsetTop", "get").mockImplementation(function (this: HTMLElement) {
    return this.tagName === "OL" ? 300 : this.dataset.match !== undefined ? 500 : 0;
  });
  const lines: LogLine[] = [{ ...head, id: "a", message: "INFO other" }, head];
  const fold = new PatternFold(); fold.add(head);
  const demo = createDemoAdapter();
  renderWithApp(<PatternSlice kind="container" id="example" pattern={fold.snapshot()[0]!} slice={{ start: Date.parse(head.ts), end: Date.parse(head.ts) + 1000 }} close={() => {}} />, { ...demo, readLogs: async (_ref, _options, sink) => { sink(lines); return { reason: "end", lines: 2, bytes: 40 }; } });
  await act(settleLogs);
  expect(screen.getByRole("list").scrollTop).toBe(500);
  offsets.mockRestore();
});

it("reads the next page from the bin start with one more page of budget, without repeating lines", async () => {
  setAppLocale("en-US");
  const start = Date.parse(head.ts);
  const at = (i: number, ms: number, message: string): LogLine => ({ id: `p${i}`, service: "web", ts: new Date(start + ms).toISOString(), level: "info", message });
  const first = [...Array.from({ length: 498 }, (_, i) => at(i, i, `INFO line ${i}`)), at(498, 600, "INFO tick a"), at(499, 600, "INFO tick b")];
  const second = [at(0, 600, "INFO tick a"), at(1, 600, "INFO tick b"), at(2, 600, "INFO tick c"), at(3, 700, "INFO late")];
  const calls: LogReadOptions[] = [];
  const fold = new PatternFold(); fold.add(first[0]!);
  const demo = createDemoAdapter();
  renderWithApp(<PatternSlice kind="container" id="example" pattern={fold.snapshot()[0]!} slice={{ start, end: start + 1000 }} close={() => {}} />, { ...demo, readLogs: async (_ref, options, sink) => {
    calls.push(options);
    const page = calls.length === 1 ? first : second;
    sink(page);
    return { reason: calls.length === 1 ? "cap-lines" : "end", lines: page.length, bytes: 100 };
  } });
  await act(settleLogs);
  expect(calls[0]).toMatchObject({ since: new Date(start).toISOString(), maxLines: 500 });
  expect(screen.getByText("Showing the first 500 lines; this period has more.")).toBeDefined();
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" }));
  await act(settleLogs);
  const shownBytes = first.reduce((sum, l) => sum + new TextEncoder().encode(l.message).length, 0);
  expect(calls[1]).toMatchObject({ since: new Date(start).toISOString(), until: "2026-01-02T03:04:05.999999999Z", maxLines: 1000, maxBytes: shownBytes + 1024 * 1024 });
  expect([screen.getAllByText("INFO tick a").length, screen.getAllByText("INFO tick c").length, screen.getAllByRole("listitem").length]).toEqual([1, 1, 502]);
  expect(screen.queryByRole("button", { name: "Load more lines" })).toBeNull();
});

it("keeps long lines on one row and scrolls them sideways", async () => {
  setAppLocale("en-US");
  const fold = new PatternFold(); fold.add(head);
  const long = { ...head, message: `ERROR failure ${"x".repeat(400)}` };
  const demo = createDemoAdapter();
  renderWithApp(<PatternSlice kind="container" id="example" pattern={fold.snapshot()[0]!} slice={{ start: Date.parse(head.ts), end: Date.parse(head.ts) + 1000 }} close={() => {}} />, { ...demo, readLogs: async (_ref, _options, sink) => { sink([long]); return { reason: "end", lines: 1, bytes: 420 }; } });
  await act(settleLogs);
  const row = screen.getByRole("listitem"), list = screen.getByRole("list");
  expect([row.classList.contains("whitespace-pre"), row.classList.contains("break-all"), list.classList.contains("overflow-auto")]).toEqual([true, false, true]);
});

it("says when the pattern read was interrupted instead of presenting it as complete", async () => {
  setAppLocale("en-US");
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const demo = createDemoAdapter();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, { ...demo, readLogs: async (_ref, _options, sink) => { sink(PATTERN_LINES.slice(0, 10)); return { reason: "interrupted", lines: 10, bytes: 100 }; } });
  await act(async () => { await Promise.resolve(); });
  while (callbacks.length) act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  expect(screen.getByRole("alert").textContent).toBe("Reading was interrupted. Only the covered time span shown above was grouped; read again for the rest.");
});

it("says when a bin read was interrupted and lets it continue", async () => {
  setAppLocale("en-US");
  const fold = new PatternFold(); fold.add(head);
  const demo = createDemoAdapter();
  renderWithApp(<PatternSlice kind="container" id="example" pattern={fold.snapshot()[0]!} slice={{ start: Date.parse(head.ts), end: Date.parse(head.ts) + 1000 }} close={() => {}} />, { ...demo, readLogs: async (_ref, _options, sink) => { sink([head]); return { reason: "interrupted", lines: 1, bytes: 13 }; } });
  await act(settleLogs);
  expect(screen.getByRole("alert").textContent).toBe("Reading this period was interrupted after the first line.");
  expect(screen.getByRole("button", { name: "Load more lines" })).toBeDefined();
});

it("shows log times to the second and one instant for a span inside one second", () => {
  setAppLocale("en-US");
  const stamp = formatLogStamp(T0 + 123);
  expect([/\d[.,]\d{3}\b/.test(stamp), formatLogSpan(T0, T0 + 900) === formatLogStamp(T0), formatLogSpan(T0, T0 + 2000).includes(" – ")]).toEqual([false, true, true]);
});
