// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original regression evidence for pattern bin paging through the real finite reader (M47 K-7 review probes).

import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { logFixture, settleLogs } from "@/data/http/logs.test-support";
import { renderWithApp } from "@/test/render";
import { PatternSlice } from "./PatternSlice";
import { PatternFold } from "./patterns";
const start = Date.parse("2026-01-02T03:04:05.000Z");
const stamp = (ms: number) => new Date(start + ms).toISOString();
const fold = new PatternFold();
fold.add({ id: "head", service: "a", ts: stamp(0), level: "info", message: "INFO head" });
afterEach(() => { setAppLocale("de"); vi.unstubAllGlobals(); });
function mount(kind: "container" | "stack") {
  setAppLocale("en-US");
  const fixture = logFixture();
  renderWithApp(<PatternSlice kind={kind} id="0~example" pattern={fold.snapshot()[0]!} slice={{ start, end: start + 1000 }} close={() => {}} />, { ...createDemoAdapter(), readLogs: fixture.owner.readLogs });
  return fixture;
}
it("reaches all 40 producer-sized lines in a byte-capped millisecond", async () => {
  const f = mount("container"); await act(settleLogs);
  act(() => f.sockets[0]!.onopen?.(new Event("open")));
  const send = (socket: typeof f.sockets[number]) => {
    for (let i = 0; i < 40; i++) socket.send({ seq: i + 1, timestamp: stamp(0), service: "a", message: `INFO ${String(i).padStart(2, "0")} ${"x".repeat(32760)}` });
    socket.disconnect();
  };
  act(() => send(f.sockets[0]!)); await act(settleLogs);
  expect(screen.getAllByRole("listitem")).toHaveLength(32);
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" })); await act(settleLogs);
  act(() => f.sockets[1]!.onopen?.(new Event("open")));
  act(() => send(f.sockets[1]!)); await act(settleLogs);
  expect(screen.getAllByRole("listitem")).toHaveLength(40);
});
it("does not discard an older stack source that arrives after a newer source", async () => {
  const f = mount("stack"); await act(settleLogs);
  act(() => f.sockets[0]!.onopen?.(new Event("open")));
  act(() => {
    for (let i = 0; i < 500; i++) f.sockets[0]!.send({ seq: i + 1, timestamp: stamp(100 + i), service: "a", message: `INFO a ${i}` });
  }); await act(settleLogs);
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" })); await act(settleLogs);
  act(() => f.sockets[1]!.onopen?.(new Event("open")));
  const since = Date.parse(new URL(f.urls[1]!).searchParams.get("since")!);
  // Each source is ordered; their concurrent merge need not be. Model the actual Since filter.
  act(() => {
    const records = [ ...Array.from({ length: 500 }, (_, i) => ({ timestamp: stamp(100 + i), service: "a", message: `INFO a ${i}` })), { timestamp: stamp(50), service: "b", message: "INFO older b" } ];
    records.filter(r => Date.parse(r.timestamp) >= since).forEach((r, i) => f.sockets[1]!.send({ ...r, seq: i + 1 }));
    f.sockets[1]!.disconnect();
  }); await act(settleLogs);
  expect(screen.queryByText("INFO older b") !== null).toBe(true);
  expect(screen.getAllByRole("listitem")[0]!.textContent?.includes("INFO older b")).toBe(true);
});
it("does not duplicate a newer stack source when the page ends in an older source", async () => {
  const f = mount("stack"); await act(settleLogs);
  act(() => f.sockets[0]!.onopen?.(new Event("open")));
  act(() => {
    for (let i = 0; i < 499; i++) f.sockets[0]!.send({ seq: i + 1, timestamp: stamp(100 + i), service: "a", message: `INFO a ${i}` });
    f.sockets[0]!.send({ seq: 500, timestamp: stamp(50), service: "b", message: "INFO b" });
  }); await act(settleLogs);
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" })); await act(settleLogs);
  act(() => f.sockets[1]!.onopen?.(new Event("open")));
  act(() => {
    for (let i = 0; i < 499; i++) f.sockets[1]!.send({ seq: i + 1, timestamp: stamp(100 + i), service: "a", message: `INFO a ${i}` });
    f.sockets[1]!.send({ seq: 500, timestamp: stamp(50), service: "b", message: "INFO b" });
    f.sockets[1]!.disconnect();
  }); await act(settleLogs);
  expect(screen.getAllByRole("listitem")).toHaveLength(500);
});
it("retains repeated identical lines with the right multiplicity across pages", async () => {
  const f = mount("container"); await act(settleLogs);
  act(() => f.sockets[0]!.onopen?.(new Event("open")));
  act(() => { for (let i = 0; i < 501; i++) f.sockets[0]!.send({ seq: i + 1, timestamp: stamp(0), service: "a", message: "INFO same" }); });
  await act(settleLogs);
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" })); await act(settleLogs);
  act(() => f.sockets[1]!.onopen?.(new Event("open")));
  act(() => { for (let i = 0; i < 501; i++) f.sockets[1]!.send({ seq: i + 1, timestamp: stamp(0), service: "a", message: "INFO same" }); f.sockets[1]!.disconnect(); });
  await act(settleLogs);
  expect(screen.getAllByRole("listitem")).toHaveLength(501);
  expect(screen.queryByRole("button", { name: "Load more lines" })).toBeNull();
});
it("marks a continuation across a page boundary and keeps the current scroll", async () => {
  const f = mount("container"); await act(settleLogs);
  act(() => f.sockets[0]!.onopen?.(new Event("open")));
  act(() => { for (let i = 0; i < 500; i++) f.sockets[0]!.send({ seq: i + 1, timestamp: stamp(i), service: "a", message: i === 499 ? "INFO head" : "INFO other" }); });
  await act(settleLogs);
  const list = screen.getByRole("list"); list.scrollTop = 123;
  await userEvent.setup().click(screen.getByRole("button", { name: "Load more lines" })); await act(settleLogs);
  act(() => f.sockets[1]!.onopen?.(new Event("open")));
  act(() => { f.sockets[1]!.send({ seq: 1, timestamp: stamp(499), service: "a", message: "INFO head" }); f.sockets[1]!.send({ seq: 2, timestamp: stamp(500), service: "a", message: "    at continuation" }); f.sockets[1]!.disconnect(); });
  await act(settleLogs);
  expect(screen.getByText("at continuation").closest("li")!.hasAttribute("data-match")).toBe(true);
  expect(list.scrollTop).toBe(123);
});
