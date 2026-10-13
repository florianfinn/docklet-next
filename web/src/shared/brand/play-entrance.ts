import { CONSTRUCTION_REF } from "./vendor/construction.js";
import { clampDuration, defaultDuration, motionPlan, resolveStyle } from "./vendor/logo-motion.js";
import type { LogoMotionStyle, LogoVariant } from "./vendor/logo-svg.js";

/**
 * WAAPI player for a logo rendered by renderLogoSVG; same plan as the study's
 * playEntrance (logo-next/src/docklet-logo.js), typed for the app. Every
 * element gets exactly one animation with fill "backwards", so the settled
 * logo is the unchanged SVG.
 */
export interface Entrance {
  animations: Animation[];
  finished: Promise<void>;
  style: LogoMotionStyle;
  duration: number;
}

export function entranceDuration(candidate: string, style: LogoMotionStyle, requested?: number): number {
  const resolved = resolveStyle(candidate, style);
  return clampDuration(requested ?? Number.NaN, defaultDuration(candidate, resolved), resolved);
}

export function playEntrance(svg: SVGSVGElement, motionStyle: LogoMotionStyle, duration?: number): Entrance | null {
  if (typeof svg.animate !== "function") return null;
  const { candidate = "stack", variant = "full", optical = "regular" } = svg.dataset;
  const wanted = resolveStyle(candidate, motionStyle);
  const style = wanted === "blueprint" && !svg.querySelector(".dln-bp") ? "assembly" : wanted;
  const total = entranceDuration(candidate, style, duration);
  if (style === "blueprint") {
    const height = svg.getBoundingClientRect().height;
    if (height > 0) svg.style.setProperty("--dln-k", String(Math.min(2.5, Math.max(0.35, CONSTRUCTION_REF / height))));
  }
  const animations: Animation[] = [];
  for (const step of motionPlan(candidate, variant as LogoVariant, total, { style, optical: optical === "small" ? "small" : "regular" })) {
    for (const element of svg.querySelectorAll<SVGElement>(step.selector)) {
      if (step.origin) {
        element.style.transformBox = "fill-box";
        element.style.transformOrigin = step.origin;
      }
      animations.push(element.animate(step.keyframes, { delay: step.delay, duration: step.duration, easing: "linear", fill: "backwards" }));
    }
  }
  const finished = Promise.all(animations.map((a) => a.finished)).then(
    () => undefined,
    () => undefined,
  );
  return { animations, finished, style, duration: total };
}
