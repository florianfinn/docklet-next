import { act, fireEvent, screen, within } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it } from "vitest";

import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { ContainerInfo } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { attentionItems } from "./attention";
import { fleetBreakdown, fleetUnits, problemCounts, problemItems } from "./fleet-status";
import { FleetRing } from "./FleetRing";

const online = HOSTS.find((h) => h.id === "atlas");
const offline = HOSTS.find((h) => h.status === "offline");
const base = STANDALONE[0];

function fleet(): ContainerInfo[] {
  if (!online || !offline || !base) throw new Error("fixture missing");
  const svc = (id: string, stackId: string, hostId: string, state: ContainerInfo["state"], health: ContainerInfo["health"]): ContainerInfo => ({ ...base, id, name: `${stackId}-app-1`, serviceName: "app", stackId, origin: "compose", hostId, state, health });
  return [svc("a1", "alpha", online.id, "running", "healthy"), svc("a2", "beta", online.id, "running", "unhealthy"), svc("c1", "gamma", offline.id, "running", "none"), svc("c2", "delta", offline.id, "stopped", "none")];
}

const units = () => fleetUnits([online!, offline!], [], fleet());

beforeEach(() => {
  document.documentElement.classList.add("dark");
});
afterEach(() => {
  document.documentElement.classList.remove("dark");
});

describe("fleet ring", () => {
  it("draws each stack of a disconnected host as one faint block with a red dashed outline, never as stopped or unhealthy", () => {
    const { container } = renderWithApp(<FleetRing hosts={[online!, offline!]} units={units()} label="Ring" center={{ value: "1/4", caption: "" }} />);
    const segs = [...container.querySelectorAll<SVGPathElement>("path[data-runtime]")];
    expect(segs.map((p) => p.dataset.runtime)).toEqual(["running", "unhealthy", "unreachable", "unreachable"]);
    const unreachable = segs.filter((p) => p.dataset.runtime === "unreachable");
    // The band itself is solid (no dash pattern that reads as many small blocks) and only faintly tinted.
    expect(unreachable.every((p) => !p.hasAttribute("stroke-dasharray") && p.getAttribute("stroke")!.includes("var(--destructive) 22%"))).toBe(true);
    const outlines = unreachable.map((p) => p.parentElement?.querySelector('path[data-outline="unreachable"]'));
    expect(outlines.every((o) => o?.getAttribute("stroke") === "var(--destructive)" && !!o.getAttribute("stroke-dasharray") && o.getAttribute("pointer-events") === "none")).toBe(true);
    expect(container.querySelectorAll('path[data-outline]').length).toBe(2);
    const arc = container.querySelector('path[data-host-arc="offline"]');
    expect([arc?.getAttribute("stroke"), !!arc?.getAttribute("stroke-dasharray")]).toEqual(["var(--destructive)", true]);
    // No native tooltip anymore.
    expect(container.querySelectorAll("title").length).toBe(0);
  });

  it("opens a rich popup on focus and moves with the arrow keys", async () => {
    const { container } = renderWithApp(<FleetRing hosts={[online!, offline!]} units={units()} label="Ring" center={{ value: "1/4", caption: "" }} />);
    const segs = [...container.querySelectorAll<SVGPathElement>("path[data-runtime]")];
    expect(segs.map((p) => p.getAttribute("tabindex"))).toEqual(["0", "-1", "-1", "-1"]);
    act(() => segs[0]?.focus());
    const first = await screen.findByRole("tooltip", { name: "Details zu alpha" });
    // Aggregated state badge plus the member list with the service's own state.
    expect(within(first).getAllByText("Läuft").length === 2 && within(first).getByText(online!.name) !== null && within(first).getByRole("list", { name: "Container" }) !== null).toBe(true);

    // Stacks are ordered by name per host: alpha, beta | delta, gamma. Left wraps to gamma, then delta.
    fireEvent.keyDown(segs[0] as SVGPathElement, { key: "ArrowLeft" });
    await screen.findByRole("tooltip", { name: "Details zu gamma" });
    fireEvent.keyDown(segs[3] as SVGPathElement, { key: "ArrowLeft" });
    const last = await screen.findByRole("tooltip", { name: "Details zu delta" });
    // Last known state, explicitly not confirmed.
    expect(within(last).getByText("Host getrennt") !== null).toBe(true);
    expect(within(last).getByText("Gestoppt (letzter Stand)") !== null).toBe(true);
    expect(within(last).getByText(/ob die Container gerade laufen, ist nicht bestätigt/) !== null).toBe(true);
    expect(document.activeElement === segs[2]).toBe(true);
  });

  it("counts a disconnected host once as a problem and keeps vulnerabilities separate", () => {
    const containers = fleet();
    const items = attentionItems({ hosts: [online!, offline!], stacks: [], containers, runs: [], images: [{ ref: "x/y", hostId: online!.id, usage: "running", counts: { critical: 2, high: 0, medium: 0, low: 0 }, scannedAt: "" }], metrics: undefined, now: Date.now() });
    expect(problemCounts(items)).toEqual({ offline: 1 });
    expect(problemItems(items).length).toBe(1);
    expect(items.some((i) => i.kind === "critical")).toBe(true);
    const b = fleetBreakdown([online!, offline!], units());
    expect([b.running, b.problem, b.unreachable, b.stopped, b.hostsOffline, b.containersUnreachable]).toEqual([1, 1, 2, 0, 1, 2]);
  });
});

describe("fleet units", () => {
  it("groups containers by stack and standalone containers by origin, with honest aggregated states", () => {
    const h = online!;
    const c = (id: string, extra: Partial<ContainerInfo>): ContainerInfo => ({ ...base!, id, name: id, hostId: h.id, state: "running", health: "none", stackId: undefined, origin: "standalone", ...extra });
    const list = [
      c("web-1", { stackId: "shop", origin: "compose", serviceName: "web" }),
      c("db-1", { stackId: "shop", origin: "compose", serviceName: "db", state: "stopped" }),
      c("api-1", { stackId: "api", origin: "compose", serviceName: "api", health: "unhealthy" }),
      c("api-2", { stackId: "api", origin: "compose", serviceName: "worker" }),
      c("tool-a", {}),
      c("tool-b", {}),
      c("legacy", { origin: "unraid" }),
    ];
    const units = fleetUnits([h], [{ id: "shop", name: "Shop", hostId: h.id, labels: [], services: [], autoUpdate: { mode: "check" } }], list);
    // Two stacks (by name), one Run group with both docker-run containers, one Unraid group.
    expect(units.map((u) => [u.kind, u.name, u.containers.length, u.state])).toEqual([
      ["stack", "api", 2, "unhealthy"],
      ["stack", "Shop", 2, "partial"],
      ["group", "standalone", 2, "running"],
      ["group", "unraid", 1, "running"],
    ]);
    // A disconnected host never reports a current state for its stacks.
    expect(fleetUnits([offline!], [], [{ ...list[0]!, hostId: offline!.id }]).map((u) => u.state)).toEqual(["unreachable"]);
  });
});
