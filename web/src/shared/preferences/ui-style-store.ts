import { useEffect } from "react";

import { createStore, mergeShallow, useStore } from "@/shared/lib/store";

import { SHIPPED_DESIGN } from "./shipped-design";

/** Visual style of segmented toggles, switches and sliders; compared live in the design lab. */
export type ToggleStyle = "pill" | "glow" | "minimal";
/** Container CPU/RAM cells: circular gauges or the slim bar with text. */
export type MetricDisplay = "rings" | "bars";
/** Sidebar host filter design; all three edit the same selection. */
export type HostFilterStyle = "list" | "chips" | "menu";
/** Look of the progressive action toast top right; all three keep their size from start to result. */
export type ToastDesign = "bar" | "compact" | "edge";
/** Layout of the centred dialog that major actions (deploy, update, restore …) open. */
export type ActionDialogDesign = "timeline" | "console" | "focus";
/** Animation of the running segment in step bars; reduced motion shows a static partial fill. */
export type RunMotion = "sweep" | "stripes" | "pulse";
/** Design-lab override of the hub setting for major actions; "hub" follows the saved hub settings. */
export type ActionPresentationOverride = "hub" | "dialog" | "toast";

export interface UiStyle {
  toggleStyle: ToggleStyle;
  metricDisplay: MetricDisplay;
  hostFilterStyle: HostFilterStyle;
  /** Pointer-following border glow (edge light). */
  pointerGlow: boolean;
  /** 0.2 … 1 */
  pointerGlowIntensity: number;
  /** Width of action and notification toasts relative to 23 rem, 0.9 … 1.6; text keeps its normal size. */
  toastScale: number;
  toastDesign: ToastDesign;
  actionDialogDesign: ActionDialogDesign;
  /** Development override of the hub's action presentation for this browser only. */
  actionPresentationOverride: ActionPresentationOverride;
  runMotion: RunMotion;
}

export const TOAST_SCALE = { min: 0.9, max: 1.6, default: 1.3 } as const;

/** How much an action toast shows at a given width: the step bar, plus key figures, plus step names under the bar. */
export type ToastDetail = "compact" | "steps" | "full";

export function toastDetail(scale: number): ToastDetail {
  return scale < 1.2 ? "compact" : scale < 1.45 ? "steps" : "full";
}

const FACTORY_UI_STYLE: UiStyle = {
  toggleStyle: "pill",
  metricDisplay: "rings",
  hostFilterStyle: "list",
  pointerGlow: false,
  pointerGlowIntensity: 0.6,
  toastScale: TOAST_SCALE.default,
  toastDesign: "bar",
  actionDialogDesign: "focus",
  actionPresentationOverride: "hub",
  runMotion: "sweep",
};

const clampNumber = (v: unknown, lo: number, hi: number, fallback: number) => (typeof v === "number" && Number.isFinite(v) ? Math.min(hi, Math.max(lo, v)) : fallback);

const TOGGLE_STYLES: readonly ToggleStyle[] = ["pill", "glow", "minimal"];
const METRIC_DISPLAYS: readonly MetricDisplay[] = ["rings", "bars"];
const HOST_FILTER_STYLES: readonly HostFilterStyle[] = ["list", "chips", "menu"];
export const TOAST_DESIGNS: readonly ToastDesign[] = ["bar", "compact", "edge"];
export const ACTION_DIALOG_DESIGNS: readonly ActionDialogDesign[] = ["focus", "timeline", "console"];
export const RUN_MOTIONS: readonly RunMotion[] = ["sweep", "stripes", "pulse"];
export const ACTION_PRESENTATION_OVERRIDES: readonly ActionPresentationOverride[] = ["hub", "dialog", "toast"];

/** Effective presentation: the design-lab override wins, otherwise the hub setting. */
export function effectivePresentation(override: ActionPresentationOverride, hub: "dialog" | "toast" | undefined): "dialog" | "toast" {
  return override === "hub" ? (hub ?? "dialog") : override;
}

/** Earlier stores kept a browser-wide on/off switch; "off" becomes the matching override. */
function presentationOverride(stored: unknown, fallback: ActionPresentationOverride): ActionPresentationOverride {
  const s = (stored && typeof stored === "object" ? stored : {}) as { actionPresentationOverride?: unknown; actionDialogAuto?: unknown };
  if (ACTION_PRESENTATION_OVERRIDES.includes(s.actionPresentationOverride as ActionPresentationOverride)) return s.actionPresentationOverride as ActionPresentationOverride;
  return s.actionDialogAuto === false ? "toast" : fallback;
}

/** Known keys only; enums must match and numbers are clamped, anything else falls back to `defaults`. */
export function mergeUiStyle(stored: unknown, defaults: UiStyle): UiStyle {
  const s = mergeShallow(stored, defaults);
  return {
    toggleStyle: TOGGLE_STYLES.includes(s.toggleStyle) ? s.toggleStyle : defaults.toggleStyle,
    metricDisplay: METRIC_DISPLAYS.includes(s.metricDisplay) ? s.metricDisplay : defaults.metricDisplay,
    hostFilterStyle: HOST_FILTER_STYLES.includes(s.hostFilterStyle) ? s.hostFilterStyle : defaults.hostFilterStyle,
    pointerGlow: s.pointerGlow === true,
    pointerGlowIntensity: clampNumber(s.pointerGlowIntensity, 0.2, 1, defaults.pointerGlowIntensity),
    toastScale: clampNumber(s.toastScale, TOAST_SCALE.min, TOAST_SCALE.max, defaults.toastScale),
    toastDesign: TOAST_DESIGNS.includes(s.toastDesign) ? s.toastDesign : defaults.toastDesign,
    actionDialogDesign: ACTION_DIALOG_DESIGNS.includes(s.actionDialogDesign) ? s.actionDialogDesign : defaults.actionDialogDesign,
    actionPresentationOverride: presentationOverride(stored, defaults.actionPresentationOverride),
    runMotion: RUN_MOTIONS.includes(s.runMotion) ? s.runMotion : defaults.runMotion,
  };
}

/** Shipped default: the favourite design's UI style; missing fields fall back to the factory values. */
export const DEFAULT_UI_STYLE: UiStyle = mergeUiStyle(SHIPPED_DESIGN.uiStyle, FACTORY_UI_STYLE);

export const uiStyleStore = createStore<UiStyle>(DEFAULT_UI_STYLE, {
  key: "docklet-react-opus.ui-style",
  version: 1,
  merge: mergeUiStyle,
});

export function useUiStyle<K extends keyof UiStyle>(key: K): UiStyle[K] {
  return useStore(uiStyleStore, (s) => s[key]);
}

export function setUiStyle(patch: Partial<UiStyle>): void {
  uiStyleStore.set((s) => ({ ...s, ...patch }));
}

/** Mirrors the toggle style and the toast width on <html> so CSS styles every control and toast at once. */
export function useApplyUiStyle(): void {
  const toggleStyle = useUiStyle("toggleStyle");
  const toastScale = useUiStyle("toastScale");
  useEffect(() => {
    document.documentElement.dataset.toggleStyle = toggleStyle;
  }, [toggleStyle]);
  useEffect(() => {
    document.documentElement.style.setProperty("--toast-scale", String(toastScale));
  }, [toastScale]);
}
