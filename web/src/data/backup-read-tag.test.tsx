// Source: docklet-next/issues/389
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original proof that only interval refreshes of backup data are polls; mount, invalidation and remount read standby hosts (Q-5a, R1E-3).

import { useQueryClient } from "@tanstack/react-query";
import { act, fireEvent, screen } from "@testing-library/react";
import { useState } from "react";
import { afterEach, expect, it, vi } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { renderWithApp } from "@/test/render";
import { useBackupOverview } from "./backup-queries";

afterEach(() => { vi.useRealTimers(); });

function Overview() {
  useBackupOverview("first");
  const client = useQueryClient();
  return <button type="button" onClick={() => void client.invalidateQueries({ queryKey: ["backup-overview", "first"] })}>invalidate</button>;
}
function Toggle() {
  const [shown, setShown] = useState(true);
  return <><button type="button" onClick={() => setShown((v) => !v)}>toggle</button>{shown && <Overview />}</>;
}

it("R1E-3 mount, invalidation and a remount after older data are background reads; an interval refresh is a poll", async () => {
  vi.useFakeTimers({ shouldAdvanceTime: true });
  const demo = createDemoAdapter();
  const tags: unknown[] = [];
  const getBackupOverview = vi.fn(async (host: string, tag?: unknown) => { tags.push(tag); return demo.getBackupOverview(host); });
  renderWithApp(<Toggle />, { ...demo, getBackupOverview } as typeof demo);
  const settle = async (ms: number) => { await act(async () => { await vi.advanceTimersByTimeAsync(ms); }); };
  await settle(10);
  fireEvent.click(screen.getByRole("button", { name: "invalidate" })); await settle(10);
  await settle(5_000);
  fireEvent.click(screen.getByRole("button", { name: "toggle" })); await settle(31_000);
  fireEvent.click(screen.getByRole("button", { name: "toggle" })); await settle(10);
  expect(tags).toEqual(["background", "background", "poll", "background"]);
});
