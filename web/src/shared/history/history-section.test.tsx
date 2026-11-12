// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original history section rendering tests for issues #335 and #336.

import { fireEvent, screen, waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { demoHistory } from "@/data/demo/demo-history";
import type { HistoryBucket, HistoryRange, RuntimeHistory } from "@/domain/history";
import { renderWithApp } from "@/test/render";

import { ContainerHistory } from "./HistorySection";

const minutes = { running: 0, unhealthy: 0, down: 0, stopped: 0 };

function fixed(uptimePercent: number | null, buckets: HistoryBucket[]): RuntimeHistory {
  return { range: "24h", buckets, uptimePercent, totals: { netRx: 2048, netTx: 1024, diskRead: 0, diskWrite: 0 } };
}

function adapterWith(history: (range: HistoryRange) => RuntimeHistory) {
  const adapter = createDemoAdapter();
  return { ...adapter, getContainerHistory: async (_ref: unknown, range: HistoryRange) => history(range) };
}

const strip = (container: HTMLElement) => [...container.querySelectorAll<HTMLElement>('[role="list"] > [role="listitem"]')];

describe("history section", () => {
  it("draws one bar per slot and switches the range", async () => {
    const { container } = renderWithApp(<ContainerHistory hostId="h" name="a" />, adapterWith((r) => demoHistory("c:a", r, Date.parse("2026-11-03T10:00:00Z"))));
    await waitFor(() => expect(strip(container).length).toBe(96));
    fireEvent.click(screen.getByRole("radio", { name: "7 Tage" }));
    await waitFor(() => expect(strip(container).length).toBe(84));
    fireEvent.click(screen.getByRole("radio", { name: "30 Tage" }));
    await waitFor(() => expect(strip(container).length).toBe(90));
    expect(container.querySelectorAll('[data-metric="cpu"] > div:last-child > *').length).toBe(90);
  });

  it("colours bars by state and the uptime by threshold, and shows no data without counted minutes", async () => {
    const at = (i: number, state: HistoryBucket["state"]): HistoryBucket => ({ start: new Date(Date.UTC(2026, 10, 3, 0, i * 15)).toISOString(), end: new Date(Date.UTC(2026, 10, 3, 0, i * 15 + 15)).toISOString(), state, minutes: { ...minutes, ...(state === "nodata" ? {} : { [state]: 15 }) } });
    const buckets = (["running", "unhealthy", "down", "stopped", "nodata"] as const).map((s, i) => at(i, s));
    const cases: [number | null, string][] = [[99.5, "ok"], [97, "warn"], [80, "err"], [null, "none"]];
    for (const [uptime, tone] of cases) {
      const { container, unmount } = renderWithApp(<ContainerHistory hostId="h" name="a" />, adapterWith(() => fixed(uptime, buckets)));
      await waitFor(() => expect(container.querySelector("[data-uptime]")).not.toBeNull());
      expect([uptime, container.querySelector<HTMLElement>("[data-uptime]")!.dataset.uptime]).toEqual([uptime, tone]);
      unmount();
    }
    const { container } = renderWithApp(<ContainerHistory hostId="h" name="a" />, adapterWith(() => fixed(100, buckets)));
    await waitFor(() => expect(strip(container).length).toBe(5));
    expect(strip(container).map((b) => [b.dataset.state, b.dataset.tone ?? "none"])).toEqual([["running", "ok"], ["unhealthy", "warn"], ["down", "err"], ["stopped", "gray"], ["nodata", "none"]]);
  });

  it("explains a stopped slot as not counted and names the exit code of a down slot", async () => {
    const b = (state: HistoryBucket["state"], m: Partial<HistoryBucket["minutes"]>, extra: Partial<HistoryBucket> = {}): HistoryBucket => ({ start: "2026-11-03T10:00:00Z", end: "2026-11-03T10:15:00Z", state, minutes: { ...minutes, ...m }, ...extra });
    const { container } = renderWithApp(<ContainerHistory hostId="h" name="a" />, adapterWith(() => fixed(50, [b("stopped", { running: 5, stopped: 10 }), { ...b("down", { running: 12, down: 3 }, { exitCode: 137 }), start: "2026-11-03T10:15:00Z", end: "2026-11-03T10:30:00Z" }])));
    await waitFor(() => expect(strip(container).length).toBe(2));
    fireEvent.focus(strip(container)[0]!);
    const stopped = await waitFor(() => document.querySelector<HTMLElement>('[data-bucket-tip="stopped"]')!);
    expect([/zählt nicht als Ausfall/.test(stopped.textContent!), /Läuft: 5 Min\./.test(stopped.textContent!), /Gestoppt: 10 Min\./.test(stopped.textContent!)]).toEqual([true, true, true]);
    fireEvent.blur(strip(container)[0]!);
    fireEvent.focus(strip(container)[1]!);
    const down = await waitFor(() => document.querySelector<HTMLElement>('[data-bucket-tip="down"]')!);
    expect([/Exit-Code 137/.test(down.textContent!), /zählt nicht/.test(down.textContent!)]).toEqual([true, false]);
  });

  it("shows resource summaries and the four totals", async () => {
    const { container } = renderWithApp(<ContainerHistory hostId="h" name="a" />, adapterWith((r) => demoHistory("c:a", r, Date.parse("2026-11-03T10:00:00Z"))));
    await waitFor(() => expect(container.querySelectorAll("[data-total]").length).toBe(4));
    expect([...container.querySelectorAll<HTMLElement>("[data-total]")].map((e) => e.dataset.total)).toEqual(["netRx", "netTx", "diskRead", "diskWrite"]);
    expect(/Spitze/.test(container.querySelector('[data-metric="cpu"]')!.textContent!)).toBe(true);
    expect(/Limit/.test(container.querySelector('[data-metric="memory"]')!.textContent!)).toBe(true);
  });
});
