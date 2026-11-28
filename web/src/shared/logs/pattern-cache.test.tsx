// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original regression evidence for remembered pattern windows and the selectable line limit (M47 K-8).

import { StrictMode } from "react";
import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { LogReadOptions, LogReadResult } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
import { registerLogLifetime } from "@/data/log-buffer";
import type { LogLine } from "@/domain/model";
import { renderWithApp } from "@/test/render";
import { PatternMode } from "./PatternMode";

afterEach(() => { setAppLocale("de"); vi.unstubAllGlobals(); });
const T0 = Date.parse("2026-01-02T03:00:00Z");
const LINES: LogLine[] = Array.from({ length: 5 }, (_, i) => ({ id: String(i), service: "web", ts: new Date(T0 + i * 1000).toISOString(), level: "info", message: `GET /a ${i}` }));

function setup(reason: LogReadResult["reason"] = "end") {
  setAppLocale("en-US");
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const calls: LogReadOptions[] = [];
  const demo = createDemoAdapter();
  const adapter: typeof demo = { ...demo, readLogs: vi.fn(async (_ref, options, sink) => { calls.push(options); sink(LINES); return { reason, lines: LINES.length, bytes: 40 }; }) };
  const drain = async () => {
    await act(async () => { await Promise.resolve(); });
    while (callbacks.length) act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  };
  return { demo, adapter, calls, drain };
}
const radio = (name: string) => screen.getByRole("radio", { name });

it("shows a remembered window again without reading it a second time", async () => {
  const { adapter, calls, drain } = setup();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain();
  const user = userEvent.setup();
  await user.click(radio("7 d")); await drain();
  await user.click(radio("24 h")); await drain();
  expect([calls.length, screen.queryByText("Reading and grouping…") === null, screen.getAllByRole("group", { name: "Distribution across 30 time bins in the covered span" }).length]).toEqual([2, true, 1]);
  await user.click(screen.getByRole("button", { name: "Read again" })); await drain();
  expect(calls.length).toBe(3);
});

it("does not remember an interrupted read", async () => {
  const { adapter, calls, drain } = setup("interrupted");
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain();
  const user = userEvent.setup();
  await user.click(radio("7 d")); await drain();
  await user.click(radio("24 h")); await drain();
  expect(calls.length).toBe(3);
});

it("never shows a remembered window to another identity", async () => {
  const { demo, adapter, calls, drain } = setup();
  let version = 1;
  registerIdentityLifetime(demo.subscribeChanges, { version: () => version, active: () => true, listen: () => () => {} });
  const first = renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain(); first.unmount();
  version = 2;
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain();
  expect(calls.length).toBe(2);
});

it("reads with the line limit chosen on the lines badge", async () => {
  const { adapter, calls, drain } = setup();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain();
  expect(calls[0]).toMatchObject({ tail: 100_000, maxLines: 100_000 });
  const user = userEvent.setup();
  await user.click(screen.getByRole("button", { name: "5 lines read, limit 100,000 lines" }));
  await user.click(within(await screen.findByRole("menu")).getByRole("menuitemradio", { name: "At most 10,000 lines" }));
  await drain();
  expect(calls.at(-1)).toMatchObject({ tail: 10_000, maxLines: 10_000 });
  expect(screen.getByRole("button", { name: "5 lines read, limit 10,000 lines" })).toBeDefined();
});
it("retains a refreshed result across leaving and reopening Patterns", async () => {
  const { adapter, calls, drain } = setup();
  const first = renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter);
  await drain();
  await userEvent.setup().click(screen.getByRole("button", { name: "Read again" })); await drain();
  expect(calls.length).toBe(2);
  first.unmount();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain();
  expect(calls.length).toBe(2);
});

it.each(["cap-lines", "cap-bytes"] as const)("remembers %s results across unmount", async (reason) => {
  const { adapter, calls, drain } = setup(reason);
  const first = renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain(); first.unmount();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain();
  expect(calls.length).toBe(1);
});

it("refresh leaves the other window cached", async () => {
  const { adapter, calls, drain } = setup();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain();
  const user = userEvent.setup();
  await user.click(radio("7 d")); await drain();
  await user.click(screen.getByRole("button", { name: "Read again" })); await drain();
  await user.click(radio("24 h")); await drain();
  await user.click(radio("7 d")); await drain();
  expect(calls.length).toBe(3);
});

it("bounds the cache to eight entries", async () => {
  const { adapter, calls, drain } = setup();
  for (let i = 0; i < 9; i++) {
    const view = renderWithApp(<PatternMode kind="container" id={String(i)} select={() => {}} />, adapter); await drain(); view.unmount();
  }
  const hit = renderWithApp(<PatternMode kind="container" id="1" select={() => {}} />, adapter); await drain(); hit.unmount();
  expect(calls.length).toBe(9);
  renderWithApp(<PatternMode kind="container" id="0" select={() => {}} />, adapter); await drain();
  expect(calls.length).toBe(10);
});

it("a completed read survives StrictMode remount", async () => {
  const { adapter, calls, drain } = setup();
  const first = renderWithApp(<StrictMode><PatternMode kind="container" id="example" select={() => {}} /></StrictMode>, adapter); await drain();
  const initial = calls.length; first.unmount();
  renderWithApp(<StrictMode><PatternMode kind="container" id="example" select={() => {}} /></StrictMode>, adapter); await drain();
  expect(calls.length).toBe(initial);
  expect(calls.every((call) => call.maxBytes === 20 * 1024 * 1024)).toBe(true);
});

it("opens and selects the line limit using the keyboard", async () => {
  const { adapter, calls, drain } = setup();
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain();
  const user = userEvent.setup();
  screen.getByRole("button", { name: "5 lines read, limit 100,000 lines" }).focus();
  await user.keyboard("{Enter}");
  expect(screen.getByRole("menuitemradio", { name: "At most 100,000 lines" }).getAttribute("aria-checked")).toBe("true");
  await user.keyboard("{Home}{Enter}"); await drain();
  expect(calls.at(-1)).toMatchObject({ tail: 10_000, maxLines: 10_000 });
});

it("invalidates a hit on a production-style log and identity reset", async () => {
  const { adapter, calls, drain } = setup();
  let version = 0; const listeners = new Set<() => void>();
  registerLogLifetime(adapter.subscribeLogs, { version: () => version, listen: (fn) => { listeners.add(fn); return () => { listeners.delete(fn); }; } });
  registerIdentityLifetime(adapter.subscribeChanges, { version: () => version, active: () => true, listen: () => () => {} });
  renderWithApp(<PatternMode kind="container" id="example" select={() => {}} />, adapter); await drain();
  act(() => { version++; listeners.forEach((fn) => fn()); }); await drain();
  expect(calls.length).toBe(2);
});
