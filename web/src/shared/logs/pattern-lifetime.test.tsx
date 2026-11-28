// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original client-side pattern folding, UI and regression evidence for M47.

import { act, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import { StackLogs } from "@/features/stacks/logs/StackLogs";
import { PatternMode } from "./PatternMode";
import { PatternQueue } from "./pattern-queue";
import { logFixture, settleLogs } from "@/data/http/logs.test-support";
import { registerLogLifetime } from "@/data/log-buffer";

beforeEach(() => setAppLocale("en-US"));
afterEach(() => { setAppLocale("de"); vi.restoreAllMocks(); vi.unstubAllGlobals(); });
it("closes a real socket and drops pending frames when switching to Raw", async () => {
  const demo = createDemoAdapter(), fixture = logFixture(), user = userEvent.setup();
  const stack = { ...(await demo.listStacks())[0]!, id: "0~example" };
  const scheduled: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { scheduled.push(cb); return scheduled.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const clear = vi.spyOn(PatternQueue.prototype, "clear"), slice = vi.spyOn(PatternQueue.prototype, "slice");
  renderWithApp(<StackLogs stack={stack} />, { ...demo, readLogs: fixture.owner.readLogs });
  await user.click(screen.getByRole("radio", { name: "Patterns" })); await settleLogs();
  fixture.sockets[0]!.send({ seq: 1, message: "ERROR queued", timestamp: new Date().toISOString(), level: "info" });
  expect(scheduled.length).toBeGreaterThan(0);
  await user.click(screen.getByRole("radio", { name: "Raw" }));
  expect(fixture.sockets[0]!.close).toHaveBeenCalledOnce(); expect(clear).toHaveBeenCalledOnce();
  expect((clear.mock.contexts[0] as PatternQueue).pending).toBe(0);
  act(() => scheduled.forEach((cb) => cb(0))); expect(slice).not.toHaveBeenCalled();
});
it.each(["unmount", "signout"])("clears the queue with no late folding after %s", async (event) => {
  const demo = createDemoAdapter(), fixture = logFixture(); let reset = () => {};
  registerLogLifetime(demo.subscribeLogs, { version: () => 0, listen: (notify) => { reset = notify; return () => {}; } });
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  const clear = vi.spyOn(PatternQueue.prototype, "clear"), slice = vi.spyOn(PatternQueue.prototype, "slice");
  const mounted = renderWithApp(<PatternMode kind="container" id="0~example" select={() => {}} />, { ...demo, readLogs: fixture.owner.readLogs });
  await settleLogs(); fixture.sockets[0]!.send({ seq: 1, message: "queued", timestamp: new Date().toISOString(), level: "info" });
  if (event === "unmount") mounted.unmount(); else act(reset);
  expect(clear).toHaveBeenCalled(); expect(fixture.sockets[0]!.close).toHaveBeenCalledOnce();
  act(() => callbacks.forEach((cb) => cb(0))); expect(slice).not.toHaveBeenCalled();
});
it("selects a pattern in Raw with the same key and falls back to samples when live lines do not match", async () => {
  const demo = createDemoAdapter(), fixture = logFixture(), user = userEvent.setup();
  const stack = { ...(await demo.listStacks())[0]!, id: "0~example" };
  const live = [{ id: "one", ts: new Date().toISOString(), service: "example", level: "error" as const, message: "ERROR request 99" }, { id: "two", ts: new Date().toISOString(), service: "example", level: "info" as const, message: "INFO other" }];
  const callbacks: FrameRequestCallback[] = [];
  vi.stubGlobal("requestAnimationFrame", (cb: FrameRequestCallback) => { callbacks.push(cb); return callbacks.length; });
  vi.stubGlobal("cancelAnimationFrame", vi.fn());
  renderWithApp(<StackLogs stack={stack} />, { ...demo, subscribeLogs: (_id, sink) => { sink(live); return () => {}; }, readLogs: fixture.owner.readLogs });
  await user.click(screen.getByRole("radio", { name: "Patterns" })); await settleLogs();
  fixture.sockets[0]!.send({ seq: 1, message: "ERROR request 42", timestamp: new Date().toISOString(), level: "info", service: "example" });
  act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  await user.click(screen.getByRole("button", { name: "ERROR request <number>" }));
  expect(screen.getByRole("radio", { name: "Raw" }).getAttribute("aria-checked")).toBe("true");
  expect(screen.queryAllByRole("option")).toHaveLength(1); expect(screen.queryByText("INFO other")).toBeNull();
  await user.click(screen.getByRole("button", { name: "Clear pattern filter" }));
  expect(screen.getAllByRole("option")).toHaveLength(2);
  await user.click(screen.getByRole("radio", { name: "Patterns" })); await settleLogs();
  fixture.sockets[1]!.send({ seq: 1, message: "WARN missing 42", timestamp: new Date().toISOString(), level: "info", service: "example" });
  act(() => callbacks.splice(0).forEach((cb) => cb(0)));
  await user.click(screen.getByRole("button", { name: "WARN missing <number>" }));
  expect(screen.getByText(/No loaded live lines match/)).toBeDefined(); expect(screen.getByText("WARN missing 42")).toBeDefined();
});
