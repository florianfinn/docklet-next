import { createStore, useStore } from "@/shared/lib/store";
import { SHIPPED_DESIGN } from "@/shared/preferences/shipped-design";

export const CORE_WIDGETS = ["attention", "updates", "timeline", "load", "capacity", "schedule", "security", "apps", "events"] as const;
export const SERVICE_KINDS = ["plex", "minecraft", "overseerr"] as const;
export type CoreWidgetId = (typeof CORE_WIDGETS)[number];
export type ServiceKind = (typeof SERVICE_KINDS)[number];
export type WidgetId = CoreWidgetId | ServiceKind;
export const WIDGET_IDS: readonly WidgetId[] = [...CORE_WIDGETS, ...SERVICE_KINDS];

export function isServiceWidget(id: WidgetId): id is ServiceKind {
  return (SERVICE_KINDS as readonly string[]).includes(id);
}

/**
 * Demo connection of a service widget. Only the address and the outcome of
 * the simulated connect are kept; tokens or passwords are never part of it.
 */
export interface ServiceConfig {
  address: string;
  status: "unconfigured" | "connected" | "error";
  /** Lets the demo show the error state on the next connect. */
  simulateError: boolean;
}

/** Widget size in grid cells: columns of the responsive grid and rows of WIDGET_ROW_REM. */
export interface WidgetSize {
  w: number;
  h: number;
}

export const WIDGET_ROW_REM = 5;
export const WIDGET_GAP_REM = 1;
/** Narrowest column; the grid shows as many columns as fit (one on phones, about eight at 21:9). */
export const WIDGET_COLUMN_REM = 22;
export const MAX_WIDGET_ROWS = 10;
export const MAX_WIDGET_COLUMNS = 4;
export const DEFAULT_WIDGET_SIZE: WidgetSize = { w: 1, h: 4 };

/** Smallest size at which each widget's content fits without clipping (lists scroll inside). */
export const WIDGET_MIN: Record<WidgetId, WidgetSize> = {
  attention: { w: 1, h: 3 },
  updates: { w: 1, h: 4 },
  timeline: { w: 1, h: 3 },
  load: { w: 1, h: 3 },
  capacity: { w: 1, h: 3 },
  schedule: { w: 1, h: 3 },
  security: { w: 1, h: 3 },
  apps: { w: 1, h: 2 },
  events: { w: 1, h: 3 },
  plex: { w: 1, h: 3 },
  minecraft: { w: 1, h: 3 },
  overseerr: { w: 1, h: 3 },
};

export interface DashboardLayout {
  order: WidgetId[];
  hidden: WidgetId[];
  services: Record<ServiceKind, ServiceConfig>;
  /** Only widgets resized away from DEFAULT_WIDGET_SIZE are listed. */
  sizes: Partial<Record<WidgetId, WidgetSize>>;
}

const EMPTY_SERVICE: ServiceConfig = { address: "", status: "unconfigured", simulateError: false };

/**
 * Default layout for browsers without a stored one: order, hidden widgets and
 * sizes of the shipped design (shared/preferences/shipped-design.ts). Service
 * widgets start unconfigured; they only show demo content after a connect.
 * Stored layouts are merged, never replaced (see mergeLayout).
 */
export const DEFAULT_LAYOUT: DashboardLayout = {
  order: [...SHIPPED_DESIGN.dashboard.order],
  hidden: [...SHIPPED_DESIGN.dashboard.hidden],
  services: { plex: EMPTY_SERVICE, minecraft: EMPTY_SERVICE, overseerr: EMPTY_SERVICE },
  sizes: Object.fromEntries(Object.entries(SHIPPED_DESIGN.dashboard.sizes).map(([id, size]) => [id, { ...size }])),
};

/** Whole cells between the widget's minimum and the grid limits. */
export function clampSize(id: WidgetId, size: Partial<WidgetSize> | undefined, columns: number = MAX_WIDGET_COLUMNS): WidgetSize {
  const min = WIDGET_MIN[id];
  const int = (v: unknown, fallback: number) => (typeof v === "number" && Number.isFinite(v) ? Math.round(v) : fallback);
  const maxW = Math.max(1, Math.min(MAX_WIDGET_COLUMNS, columns));
  return {
    w: Math.min(maxW, Math.max(Math.min(min.w, maxW), int(size?.w, DEFAULT_WIDGET_SIZE.w))),
    h: Math.min(MAX_WIDGET_ROWS, Math.max(min.h, int(size?.h, DEFAULT_WIDGET_SIZE.h))),
  };
}

export function widgetSize(layout: DashboardLayout, id: WidgetId): WidgetSize {
  return clampSize(id, layout.sizes[id]);
}

export function resizeWidget(layout: DashboardLayout, id: WidgetId, size: WidgetSize): DashboardLayout {
  const next = clampSize(id, size);
  const sizes = { ...layout.sizes };
  if (next.w === DEFAULT_WIDGET_SIZE.w && next.h === DEFAULT_WIDGET_SIZE.h) delete sizes[id];
  else sizes[id] = next;
  return { ...layout, sizes };
}

function sanitizeService(value: unknown): ServiceConfig {
  const v = (value && typeof value === "object" ? value : {}) as Partial<ServiceConfig>;
  const status = v.status === "connected" || v.status === "error" ? v.status : "unconfigured";
  return { address: typeof v.address === "string" ? v.address : "", status, simulateError: v.simulateError === true };
}

/** Keeps known ids in stored order, appends widgets added later and drops anything secret-looking. */
export function mergeLayout(stored: unknown, defaults: DashboardLayout): DashboardLayout {
  const s = stored as Partial<DashboardLayout> | null;
  if (!s?.order) return defaults;
  const known = (id: unknown): id is WidgetId => WIDGET_IDS.includes(id as WidgetId);
  const order = [...new Set(s.order.filter(known))];
  const added = WIDGET_IDS.filter((id) => !order.includes(id));
  const hidden = [...(s.hidden ?? []).filter(known), ...added.filter((id) => isServiceWidget(id) || defaults.hidden.includes(id))];
  const services = Object.fromEntries(SERVICE_KINDS.map((k) => [k, sanitizeService(s.services?.[k])])) as Record<ServiceKind, ServiceConfig>;
  const sizes: Partial<Record<WidgetId, WidgetSize>> = {};
  for (const [id, size] of Object.entries(s.sizes ?? {})) if (known(id)) sizes[id] = clampSize(id, size as Partial<WidgetSize>);
  return { order: [...order, ...added], hidden: [...new Set(hidden)], services, sizes };
}

export const layoutStore = createStore<DashboardLayout>(DEFAULT_LAYOUT, {
  key: "docklet-react-opus.dashboard",
  version: 1,
  merge: mergeLayout,
});

export function useLayout(): DashboardLayout {
  return useStore(layoutStore, (l) => l);
}

export function visibleWidgets(layout: DashboardLayout): WidgetId[] {
  return layout.order.filter((w) => !layout.hidden.includes(w));
}

/** Moves a widget by delta within the visible order (hidden ones keep their slot). */
export function moveWidget(layout: DashboardLayout, id: WidgetId, delta: number): DashboardLayout {
  const visible = visibleWidgets(layout);
  const i = visible.indexOf(id);
  const j = i + delta;
  if (i < 0 || j < 0 || j >= visible.length) return layout;
  const target = visible[j] as WidgetId;
  const order = [...layout.order];
  const a = order.indexOf(id);
  const b = order.indexOf(target);
  order[a] = target;
  order[b] = id;
  return { ...layout, order };
}

/** Applies a new visible order from drag and drop; hidden widgets keep their relative slots. */
export function reorderVisible(layout: DashboardLayout, visibleOrder: WidgetId[]): DashboardLayout {
  const queue = [...visibleOrder];
  const order = layout.order.map((id) => (layout.hidden.includes(id) ? id : (queue.shift() ?? id)));
  return { ...layout, order };
}

export function toggleWidget(layout: DashboardLayout, id: WidgetId): DashboardLayout {
  const hidden = layout.hidden.includes(id) ? layout.hidden.filter((w) => w !== id) : [...layout.hidden, id];
  return { ...layout, hidden };
}

export function setService(layout: DashboardLayout, kind: ServiceKind, patch: Partial<ServiceConfig>): DashboardLayout {
  return { ...layout, services: { ...layout.services, [kind]: { ...layout.services[kind], ...patch } } };
}

/** Animates layout changes with the View Transitions API when available. */
export function withTransition(update: () => void, enabled: boolean): void {
  const doc = document as Document & { startViewTransition?: (cb: () => void) => unknown };
  if (enabled && doc.startViewTransition) doc.startViewTransition(update);
  else update();
}
