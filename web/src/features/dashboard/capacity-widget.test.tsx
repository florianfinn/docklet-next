// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original capacity trend widget tests for issue #337.

import { waitFor } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import { createDemoAdapter } from "@/data/demo/demo-adapter";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { HostCapacity } from "@/domain/history";
import { renderWithApp } from "@/test/render";

import { attentionItems } from "./attention";
import { CapacityWidget } from "./widgets/SmallWidgets";

const GIB = 1024 ** 3;
const online = HOSTS.filter((h) => h.status === "online").slice(0, 2);

function trend(hostId: string, extra: Partial<HostCapacity>): HostCapacity {
  return { hostId, diskTotal: 1000 * GIB, diskUsed: 600 * GIB, series: Array.from({ length: 30 }, (_, i) => ({ at: new Date(Date.UTC(2026, 9, 5 + i)).toISOString(), used: (400 + i * 6) * GIB })), ...extra };
}

describe("capacity widget (CP1)", () => {
  it("draws the 30-day line, the runway and the growth, and says when growth is not known yet", async () => {
    const [a, b] = online;
    const adapter = { ...createDemoAdapter(), getCapacity: async () => [trend(a!.id, { growthPerDay: 2.5 * GIB, daysUntilFull: 20.4 }), trend(b!.id, { series: [] })] };
    const { container } = renderWithApp(<CapacityWidget hosts={[a!, b!]} />, adapter);
    await waitFor(() => expect(container.querySelector(`[data-capacity-host="${a!.id}"] [data-capacity-line]`)).not.toBeNull());
    const first = container.querySelector<HTMLElement>(`[data-capacity-host="${a!.id}"]`)!;
    const second = container.querySelector<HTMLElement>(`[data-capacity-host="${b!.id}"]`)!;
    expect(first.querySelector<SVGElement>("[data-capacity-line]")!.dataset.capacityLine).toBe("30");
    expect([/voll in ca\. 20 Tagen/.test(first.textContent!), first.querySelector("[data-tone]")!.getAttribute("data-tone"), first.querySelector<HTMLElement>("[data-growth]")!.dataset.growth]).toEqual([true, "err", "known"]);
    expect(/\+2,5 GB\/Tag/.test(first.textContent!)).toBe(true);
    expect([second.querySelector("[data-capacity-line]"), second.querySelector<HTMLElement>("[data-growth]")!.dataset.growth]).toEqual([null, "unavailable"]);
  });

  it("raises a disk attention item from the runway, not from the fill level alone", () => {
    const [a] = online;
    const metrics = { [a!.id]: { hostId: a!.id, cpu: 1, mem: 1, disk: 40, cpuHistory: [], memHistory: [] } };
    const base = { hosts: [a!], stacks: [], containers: [], runs: [], images: [], metrics, now: Date.now() };
    const kinds = (capacity?: HostCapacity[]) => attentionItems({ ...base, capacity }).filter((i) => i.kind === "disk").map((i) => [i.tone, i.values.days]);
    expect(kinds()).toEqual([]);
    expect(kinds([trend(a!.id, { daysUntilFull: 12 })])).toEqual([["err", 12]]);
    expect(kinds([trend(a!.id, { daysUntilFull: 45 })])).toEqual([["warn", 45]]);
    expect(kinds([trend(a!.id, {})])).toEqual([]);
  });
});
