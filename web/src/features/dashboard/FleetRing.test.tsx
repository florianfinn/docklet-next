import { describe, expect, it } from "vitest";

import { STANDALONE } from "@/data/demo/fixtures/containers";
import { HOSTS } from "@/data/demo/fixtures/hosts";
import type { ContainerInfo } from "@/domain/model";
import { renderWithApp } from "@/test/render";

import { fleetUnits } from "./fleet-status";
import { FleetRing } from "./FleetRing";
import { glowRegion, layoutRing, strokedArcBox } from "./ring-geometry";

const host = HOSTS.find((h) => h.status === "online");
const base = STANDALONE[0];

/** One single-service stack per segment, so the segment count is exact. */
function units(count: number, unhealthyAt: number[]) {
  if (!host || !base) throw new Error("fixture missing");
  const list: ContainerInfo[] = Array.from({ length: count }, (_, i) => ({ ...base, id: `c-${i}`, name: `c${i}`, stackId: `s${String(i).padStart(2, "0")}`, origin: "compose", hostId: host.id, state: "running", health: unhealthyAt.includes(i) ? "unhealthy" : "healthy" }));
  return fleetUnits([host], [], list);
}

const inside = (inner: { x: number; y: number; width: number; height: number }, outer: typeof inner) =>
  inner.x >= outer.x && inner.y >= outer.y && inner.x + inner.width <= outer.x + outer.width && inner.y + inner.height <= outer.y + outer.height;

describe("fleet ring glow", () => {
  it("keeps every stroked segment inside the glow region at any position", () => {
    const size = 220;
    const c = size / 2;
    const region = glowRegion(size, 3);
    for (const count of [1, 2, 7, 24, 61]) {
      const [group] = layoutRing([{ group: "h", items: Array.from({ length: count }, (_, i) => i) }]);
      for (const seg of group?.segments ?? []) {
        expect(inside(strokedArcBox(c, c, c - 30, seg.start, seg.end, 14), region), `${count}/${seg.index}`).toBe(true);
      }
    }
  });

  it("renders the unhealthy segment with a user-space filter that covers the whole ring", () => {
    const size = 220;
    // Segment 12 of 24 sits at 6 o'clock, where its fill box is only a few pixels tall.
    const { container } = renderWithApp(<FleetRing hosts={host ? [host] : []} units={units(24, [12])} label="Ring" center={{ value: "23/24", caption: "laufen" }} size={size} />);
    const unhealthy = container.querySelectorAll('path[data-runtime="unhealthy"]');
    expect(unhealthy.length).toBe(1);
    const ref = /^url\(#(.+)\)$/.exec(unhealthy[0]?.getAttribute("filter") ?? "")?.[1];
    const filter = ref ? container.querySelector(`filter[id="${ref}"]`) : null;
    expect(filter !== null).toBe(true);
    expect(filter?.getAttribute("filterUnits")).toBe("userSpaceOnUse");
    const region = { x: Number(filter?.getAttribute("x")), y: Number(filter?.getAttribute("y")), width: Number(filter?.getAttribute("width")), height: Number(filter?.getAttribute("height")) };
    expect(inside({ x: 0, y: 0, width: size, height: size }, region)).toBe(true);
    // All segments share one class so animation origin and hover stay identical.
    const segments = [...container.querySelectorAll('path[data-runtime]')];
    expect(segments.length === 24 && segments.every((p) => p.classList.contains("ring-seg"))).toBe(true);
  });

  it("gives each ring its own filter id", () => {
    const { container } = renderWithApp(
      <>
        <FleetRing hosts={host ? [host] : []} units={units(4, [1])} label="A" center={{ value: "", caption: "" }} />
        <FleetRing hosts={host ? [host] : []} units={units(4, [2])} label="B" center={{ value: "", caption: "" }} />
      </>,
    );
    const ids = [...container.querySelectorAll("filter")].map((f) => f.id);
    expect(ids.length === 2 && ids[0] !== ids[1]).toBe(true);
  });
});
