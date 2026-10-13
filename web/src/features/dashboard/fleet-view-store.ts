import type { Host } from "@/domain/model";
import { createStore, useStore } from "@/shared/lib/store";
import { SHIPPED_DESIGN } from "@/shared/preferences/shipped-design";

/** Experimental fleet visualisations of the dashboard summary (design lab). */
export type FleetMode = "joint" | "perHost" | "line";
/** Host cards under the ring(s) or beside them on wide screens. */
export type CardsPlacement = "below" | "right";

/**
 * Size of the host card area in cells: `w` twelfths of the summary width
 * beside the visual (placement "right"), `h` minimum height in dashboard rows.
 * `h: 1` is the flat view: cards keep their natural compact height. Cards
 * stretch to fill taller areas, so the area never leaves a blank band.
 */
export interface HostAreaSize {
  w: number;
  h: number;
}

export const HOST_AREA = { minW: 4, maxW: 9, minH: 1, maxH: 8 } as const;
export const DEFAULT_HOST_AREA: HostAreaSize = { ...SHIPPED_DESIGN.fleet.hostArea };

export function clampHostArea(size: Partial<Record<keyof HostAreaSize, unknown>> | undefined): HostAreaSize {
  const int = (v: unknown, fallback: number, min: number, max: number) => (typeof v === "number" && Number.isFinite(v) ? Math.min(max, Math.max(min, Math.round(v))) : fallback);
  return { w: int(size?.w, DEFAULT_HOST_AREA.w, HOST_AREA.minW, HOST_AREA.maxW), h: int(size?.h, DEFAULT_HOST_AREA.h, HOST_AREA.minH, HOST_AREA.maxH) };
}

export interface FleetView {
  mode: FleetMode;
  /** Diameter of the joint ring in px; per-host rings derive from it. */
  ringSize: number;
  placement: CardsPlacement;
  /** Hovered or focused ring segment moves outward towards its popup. */
  fan: boolean;
  /** Dashboard-only host order for cards, per-host rings and lines; empty = by name. */
  hostOrder: string[];
  hostArea: HostAreaSize;
}

export const RING_SIZE = { min: 160, max: 320, default: 220 } as const;
const MODES: readonly FleetMode[] = ["joint", "perHost", "line"];

const FACTORY_FLEET_VIEW: FleetView = { mode: "joint", ringSize: RING_SIZE.default, placement: "below", fan: true, hostOrder: [], hostArea: DEFAULT_HOST_AREA };

export function mergeFleetView(stored: unknown, defaults: FleetView): FleetView {
  const s = (stored && typeof stored === "object" ? stored : {}) as Partial<Record<keyof FleetView, unknown>>;
  const size = typeof s.ringSize === "number" && Number.isFinite(s.ringSize) ? Math.round(Math.min(RING_SIZE.max, Math.max(RING_SIZE.min, s.ringSize))) : defaults.ringSize;
  return {
    mode: MODES.includes(s.mode as FleetMode) ? (s.mode as FleetMode) : defaults.mode,
    ringSize: size,
    placement: s.placement === "right" || s.placement === "below" ? s.placement : defaults.placement,
    fan: typeof s.fan === "boolean" ? s.fan : defaults.fan,
    hostOrder: Array.isArray(s.hostOrder) ? [...new Set(s.hostOrder.filter((x): x is string => typeof x === "string"))] : [],
    hostArea: s.hostArea && typeof s.hostArea === "object" ? clampHostArea(s.hostArea as Partial<Record<keyof HostAreaSize, unknown>>) : defaults.hostArea,
  };
}

/** Shipped default: the favourite design's fleet section; never a host order. */
export const DEFAULT_FLEET_VIEW: FleetView = mergeFleetView({ ...SHIPPED_DESIGN.fleet, hostOrder: [] }, FACTORY_FLEET_VIEW);

export const fleetViewStore = createStore<FleetView>(DEFAULT_FLEET_VIEW, { key: "docklet-react-opus.fleet-view", version: 1, merge: mergeFleetView });

export function useFleetView(): FleetView {
  return useStore(fleetViewStore, (v) => v);
}

export function setFleetView(patch: Partial<FleetView>): void {
  fleetViewStore.set((v) => ({ ...v, ...patch }));
}

/** Hosts in the saved dashboard order; hosts not in it keep their name order after the known ones. */
export function orderHosts(hosts: readonly Host[], order: readonly string[]): Host[] {
  const rank = new Map(order.map((id, i) => [id, i]));
  return hosts.toSorted((a, b) => (rank.get(a.id) ?? Infinity) - (rank.get(b.id) ?? Infinity) || hosts.indexOf(a) - hosts.indexOf(b));
}
