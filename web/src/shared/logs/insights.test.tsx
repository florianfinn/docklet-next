// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 1829fa7776d511dab9604d9b5e3a81cfc7c55640
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original M47 Health and Logs implementation and deterministic regression evidence.

import { screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import { setAppLocale } from "@/app/i18n/locale-store";
import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import { StackLogs } from "@/features/stacks/logs/StackLogs";
import { LogDensity } from "./LogInsights";
import type { LogLine } from "@/domain/model";
const lines: LogLine[] = ["one", "two", "three", "four"].map((id, i) => Object.freeze({ id, ts: `2026-01-01T00:00:0${i}Z`, service: "jellyfin", level: i < 3 ? "warn" : "error", message: i < 3 ? "retry" : "failed" }));
beforeEach(() => setAppLocale("en-US"));
afterEach(() => setAppLocale("de"));
it("collapses filtered repeats, expands by keyboard, jumps errors and density with follow off", async () => {
  const user = userEvent.setup(), demo = createDemoAdapter();
  const stack = (await demo.listStacks())[0]!;
  renderWithApp(<StackLogs stack={stack} />, { ...demo, subscribeLogs: (_id, push) => { push(Object.freeze(lines) as LogLine[]); return () => {}; } });
  const repeat = screen.getByRole("button", { name: "Expand 3 repeats" });
  expect(screen.getAllByRole("option")).toHaveLength(2);
  repeat.focus(); await user.keyboard("{Enter}");
  expect(screen.getAllByRole("option")).toHaveLength(4);
  await user.click(screen.getByRole("button", { name: "Next error/warning" }));
  expect(screen.getByRole("button", { name: "Follow" }).getAttribute("aria-pressed")).toBe("false");
  expect(screen.getAllByRole("option")[0]!.getAttribute("aria-selected")).toBe("true");
  const bin = screen.getAllByRole("button").find((b) => b.getAttribute("aria-label")?.startsWith("1 errors, 0 warnings from"))!;
  await user.click(bin);
  expect(screen.getAllByRole("option")[3]!.getAttribute("aria-selected")).toBe("true");
});
it("renders exactly sixty density bins with only occupied warning/error bins interactive", () => {
  const select = vi.fn();
  const { container } = renderWithApp(<LogDensity lines={lines} onSelect={select} />);
  const strip = container.querySelector('[aria-label="Error and warning density"]')!;
  expect(strip.children.length).toBe(60);
  expect(strip.querySelectorAll("button").length).toBe(4);
});
