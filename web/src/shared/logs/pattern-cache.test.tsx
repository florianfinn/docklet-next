// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original regression evidence for remembered pattern windows and the selectable line limit (M47 K-8).

import { act, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import type { LogReadOptions, LogReadResult } from "@/data/api";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { registerIdentityLifetime } from "@/data/identity-lifetime";
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
