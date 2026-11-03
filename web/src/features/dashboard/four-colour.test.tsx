// Source: docklet-next/issues/371
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original four-colour status, ring layout and ring rendering tests.

import { describe, expect, it } from "vitest";

import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { ContainerInfo, RuntimeCondition, Service, Stack } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE, runtimeStatus, serviceRuntime, stackStatus, STACK_STATUS_TONE } from "@/domain/status";
import { renderWithApp } from "@/test/render";

import { fleetUnits, matchesFleetFilter, remainderStart, unitState } from "./fleet-status";
import { FleetRing } from "./FleetRing";
import { layoutRing, MIN_SEGMENT } from "./ring-geometry";

const host = HOSTS.find((h) => h.status === "online")!;
const offline = HOSTS.find((h) => h.status === "offline")!;
const base = STANDALONE[0]!;

const cond = (containerId: string, condition: RuntimeCondition["condition"], reason = "exit", exitCode?: number): RuntimeCondition => ({ containerId, condition, reason, exitCode, sampledAt: "2026-11-03T10:00:00Z" });

function ctr(id: string, stackId: string | undefined, state: ContainerInfo["state"], health: ContainerInfo["health"] = "none", condition?: RuntimeCondition, extra: Partial<ContainerInfo> = {}): ContainerInfo {
  return { ...base, id, name: id, stackId, origin: stackId ? "compose" : "standalone", hostId: host.id, state, health, condition, ...extra };
}

describe("four-colour runtime mapping (FR1)", () => {
  it("maps every live state, health and condition to one of four colours", () => {
    const rows: [ContainerInfo["state"], ContainerInfo["health"], RuntimeCondition["condition"] | undefined, string, string][] = [
      ["running", "healthy", undefined, "running", "ok"],
      ["running", "none", undefined, "running", "ok"],
      ["running", "starting", undefined, "starting", "ok"],
      ["running", "unhealthy", undefined, "unhealthy", "warn"],
      ["running", "none", "unhealthy", "restarting", "warn"],
      ["restarting", "none", undefined, "restarting", "warn"],
      ["stopped", "none", undefined, "stopped", "gray"],
      ["stopped", "none", "stopped", "stopped", "gray"],
      ["stopped", "none", "down", "down", "err"],
      ["unknown", "none", undefined, "unknown", "gray"],
    ];
    for (const [state, health, c, expected, tone] of rows) {
      const condition = c ? cond("h~x", c, c === "unhealthy" ? "restart-loop" : "exit") : undefined;
      const got = runtimeStatus(state, health, true, condition);
      expect([state, health, c, got]).toEqual([state, health, c, expected]);
      expect(RUNTIME_TONE[got]).toBe(tone);
    }
  });

  it("a restart loop followed by a Docklet stop is grey; a crash after a loop is red", () => {
    expect(runtimeStatus("stopped", "none", true, cond("h~x", "stopped", "intent", 137))).toBe("stopped");
    expect(runtimeStatus("stopped", "none", true, cond("h~x", "down", "oom", 137))).toBe("down");
  });

  it("ignores a condition sampled for another incarnation of the container", () => {
    const stale = ctr("h~new", "s1", "stopped", "none", cond("h~old", "down"));
    const fresh = ctr("h~new", "s1", "stopped", "none", cond("h~new", "down"));
    expect([containerRuntime(stale, host), containerRuntime(fresh, host)]).toEqual(["stopped", "down"]);
    const svc = (condition: RuntimeCondition): Service => ({ name: "app", containerId: "h~new", image: base.image, state: "stopped", health: "none", ports: [], networks: [], restartPolicy: "", memLimitMb: 0, restarts24h: 0, update: base.update, condition });
    expect([serviceRuntime(svc(cond("h~old", "down")), host), serviceRuntime(svc(cond("h~new", "down")), host)]).toEqual(["stopped", "down"]);
  });

  it("stack status: worst service wins, grey only when all are stopped, partly stopped stays green", () => {
    const service = (state: Service["state"], health: Service["health"] = "none", condition?: RuntimeCondition): Service => ({ name: "s", containerId: condition?.containerId, image: base.image, state, health, ports: [], networks: [], restartPolicy: "", memLimitMb: 0, restarts24h: 0, update: base.update, condition });
    const stack = (services: Service[], reportedStatus?: string): Stack => ({ id: "h~p", hostId: host.id, name: "p", services, labels: [], autoUpdate: { mode: "check" }, reportedStatus }) as Stack;
    const cases: [Stack, string][] = [
      [stack([service("running"), service("stopped")]), "running"],
      [stack([service("stopped"), service("stopped")]), "stopped"],
      [stack([service("running"), service("running", "unhealthy")]), "unhealthy"],
      [stack([service("running", "unhealthy"), service("stopped", "none", cond("h~d", "down"))]), "down"],
      [stack([service("restarting")]), "unhealthy"],
      [stack([], "stopped"), "stopped"],
      [stack([], "running"), "running"],
    ];
    for (const [s, expected] of cases) expect(stackStatus(s, host)).toBe(expected);
    expect(stackStatus(stack([service("running")]), offline)).toBe("unreachable");
    expect([STACK_STATUS_TONE.running, STACK_STATUS_TONE.unhealthy, STACK_STATUS_TONE.down, STACK_STATUS_TONE.stopped, STACK_STATUS_TONE.unreachable]).toEqual(["ok", "warn", "err", "gray", "err"]);
  });

  it("ring units aggregate the same way and carry the grey remainder and the update flag", () => {
    expect(unitState([ctr("a", "s", "running"), ctr("b", "s", "stopped")], host)).toBe("running");
    expect(unitState([ctr("a", "s", "stopped"), ctr("b", "s", "stopped")], host)).toBe("stopped");
    expect(unitState([ctr("a", "s", "running", "unhealthy"), ctr("b", "s", "stopped", "none", cond("b", "down"))], host)).toBe("down");
    const update = { state: "version", current: base.image, latest: { ...base.image, tag: "99" }, level: "minor", checkedAt: "2026-11-03T10:00:00Z", registry: "registry.example.com" } as unknown as ContainerInfo["update"];
    const [unit] = fleetUnits([host], [], [ctr("a", "s", "running"), ctr("b", "s", "stopped"), ctr("c", "s", "running", "none", undefined, { update })]);
    expect([unit!.state, unit!.running, unit!.stopped, unit!.update]).toEqual(["running", 2, 1, true]);
    expect([matchesFleetFilter(unit!, "running"), matchesFleetFilter(unit!, "update"), matchesFleetFilter(unit!, "stopped")]).toEqual([true, true, false]);
  });
});

describe("ring layout by container count (FR3)", () => {
  const overlaps = (segs: { start: number; end: number }[]) => segs.some((s, i) => i > 0 && s.start < segs[i - 1]!.end);

  it("gives larger stacks wider segments on top of a shared minimum", () => {
    const [g] = layoutRing([{ group: "h", items: [1, 9] }], (n) => n);
    const [small, large] = g!.segments;
    expect(large!.end - large!.start > 3 * (small!.end - small!.start)).toBe(true);
    expect(small!.end - small!.start >= MIN_SEGMENT - 2).toBe(true);
  });

  it("stays overlap-free from one to 150 units across many host groups with skewed counts", () => {
    for (const total of [1, 5, 40, 150]) {
      for (const hosts of [1, 4, 12]) {
        const groups = Array.from({ length: hosts }, (_, h) => ({ group: h, items: Array.from({ length: Math.ceil(total / hosts) }, (_, i) => ({ h, i, w: i === 0 ? 60 : 1 })) }));
        const layout = layoutRing(groups, (item) => item.w);
        const segs = layout.flatMap((g) => g.segments);
        expect([total, hosts, overlaps(segs)]).toEqual([total, hosts, false]);
        expect(segs.every((s) => s.end > s.start && s.end <= 360 + 1e-6)).toBe(true);
        expect(segs.map((s) => s.index)).toEqual(segs.map((_, i) => i));
      }
    }
  });
});

describe("ring rendering (FR4)", () => {
  it("draws the grey remainder of a partly stopped stack and the orange update arc", () => {
    const update = { state: "version", current: base.image, latest: { ...base.image, tag: "99" }, level: "minor", checkedAt: "2026-11-03T10:00:00Z", registry: "registry.example.com" } as unknown as ContainerInfo["update"];
    const units = fleetUnits([host], [], [ctr("a", "s1", "running"), ctr("b", "s1", "stopped"), ctr("c", "s1", "stopped"), ctr("d", "s1", "running"), ctr("e", "s2", "running", "none", undefined, { update })]);
    const { container } = renderWithApp(<FleetRing hosts={[host]} units={units} label="Ring" center={{ value: "2/2", caption: "" }} />);
    const remainders = [...container.querySelectorAll<SVGPathElement>("path[data-remainder]")];
    expect(remainders.map((p) => p.dataset.remainder)).toEqual(["2"]);
    expect(container.querySelectorAll("path[data-update-arc]").length).toBe(1);
    const s1 = units.find((u) => u.stackId === "s1")!;
    expect(remainderStart(0, 100, s1)).toBe(50);
    expect(remainderStart(0, 100, units.find((u) => u.stackId === "s2")!)).toBeUndefined();
    const segs = [...container.querySelectorAll<SVGPathElement>("path[data-runtime]")];
    expect(segs.map((p) => p.dataset.runtime)).toEqual(["running", "running"]);
    expect(/update/i.test(segs[1]!.getAttribute("aria-label")!)).toBe(true);
    expect(/update/i.test(segs[0]!.getAttribute("aria-label")!)).toBe(false);
  });

  it("dims segments outside the chip filter", () => {
    const units = fleetUnits([host], [], [ctr("a", "s1", "running"), ctr("b", "s2", "stopped", "none", cond("b", "down"))]);
    const { container } = renderWithApp(<FleetRing hosts={[host]} units={units} label="Ring" center={{ value: "1/2", caption: "" }} highlight="down" />);
    const dimmed = [...container.querySelectorAll<SVGGElement>("g[data-dimmed]")];
    expect(dimmed.length).toBe(1);
    expect(dimmed[0]!.querySelector("path[data-runtime]")!.getAttribute("data-runtime")).toBe("running");
  });
});

describe("ring chips", () => {
  it("hides empty chips, highlights on hover and pins on click", async () => {
    const { fireEvent, screen } = await import("@testing-library/react");
    const { FleetChips } = await import("./FleetChips");
    const units = fleetUnits([host], [], [ctr("a", "s1", "running"), ctr("b", "s2", "stopped", "none", cond("b", "down"))]);
    const pins: (string | null)[] = [];
    const hovers: (string | null)[] = [];
    const { container } = renderWithApp(<FleetChips units={units} pinned={null} onPin={(f) => pins.push(f)} onHover={(f) => hovers.push(f)} />);
    const keys = [...container.querySelectorAll<HTMLButtonElement>("button[data-chip]")].map((b) => b.dataset.chip);
    expect(keys).toEqual(["down", "running"]);
    const down = container.querySelector<HTMLButtonElement>('button[data-chip="down"]')!;
    expect(screen.getAllByRole("button").length).toBe(2);
    fireEvent.pointerEnter(down);
    fireEvent.click(down);
    expect([hovers[0], pins[0]]).toEqual([down.dataset.chip, down.dataset.chip]);
  });
});
