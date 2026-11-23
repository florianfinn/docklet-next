// Source: Docklet Next logo study, logo-motion/docklet-next-loader-wordmark.html (first-party)
// Revision: 172bb611b7211c9f5dab552a4aac07c6ec638d425ac499ba4b4c107f190a7201
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Motion plan only (SHA-256 of the study file above); it runs on the app logo from the vendored renderer instead of the study markup, uses palette colours from brand-logo.css instead of hex values, and stops for reduced motion and on unmount.

import { useEffect, useId, useMemo, useRef, useState } from "react";

import { cn } from "@/shared/lib/utils";

import "./brand-logo.css";
import { FAVORITE_LOGO, renderLogoSVG } from "./vendor/logo-svg.js";

/** One loop of the study loader: blocks flash, a head runs the bowl, the wordmark sweeps and NEXT answers. */
export const LOADER_CYCLE_MS = 2600;
const IO = "cubic-bezier(.65,0,.35,1)";
const REDUCED = "(prefers-reduced-motion: reduce)";

function useReducedMotion() {
  const query = typeof window !== "undefined" && typeof window.matchMedia === "function" ? window.matchMedia(REDUCED) : null;
  const [reduced, setReduced] = useState(() => query?.matches ?? false);
  useEffect(() => {
    if (!query) return;
    const change = () => setReduced(query.matches);
    query.addEventListener?.("change", change);
    return () => query.removeEventListener?.("change", change);
  }, [query]);
  return reduced;
}

/** Plays one cycle on the rendered logo and returns every animation it started. */
function playCycle(svg: SVGSVGElement, glint: SVGPathElement | null, bar: HTMLElement | null): Animation[] {
  const style = getComputedStyle(svg);
  const primary = style.getPropertyValue("--dln-c-main").trim() || "currentColor";
  const accent = style.getPropertyValue("--dln-c-next").trim() || "currentColor";
  const started: Animation[] = [];
  // Cancelling rejects `finished`; nothing awaits it, so the rejection is expected and dropped.
  const run = (el: Element, keyframes: Keyframe[], options: KeyframeAnimationOptions) => { const a = el.animate(keyframes, options); a.finished?.catch(() => {}); started.push(a); };
  ["b1", "b2", "b3"].forEach((part, k) => {
    const block = svg.querySelector(`[data-part="${part}"]`);
    if (block) run(block, [{ fill: primary }, { fill: accent, offset: 0.3 }, { fill: primary }], { duration: 520, delay: k * 110, easing: IO });
  });
  if (glint) run(glint, [{ strokeDashoffset: 0.3, opacity: 0 }, { opacity: 1, offset: 0.12 }, { opacity: 1, offset: 0.8 }, { strokeDashoffset: -1, opacity: 0 }], { duration: 480, delay: 260, easing: "cubic-bezier(.45,0,.55,1)" });
  svg.querySelectorAll(".dln-word-main .dln-letter").forEach((letter, i) => run(letter, [{ stroke: primary }, { stroke: accent, offset: 0.4 }, { stroke: primary }], { duration: 420, delay: 560 + i * 110, easing: IO }));
  svg.querySelectorAll(".dln-word-next .dln-letter").forEach((letter, i) => run(letter, [{ stroke: accent }, { stroke: primary, offset: 0.4 }, { stroke: accent }], { duration: 360, delay: 1420 + i * 90, easing: IO }));
  if (bar) run(bar, [{ transform: "translateX(-100%)" }, { transform: "translateX(250%)" }], { duration: 2000, easing: "cubic-bezier(.45,0,.55,1)" });
  return started;
}

interface BrandLoaderProps {
  /** Height of the logo in px. */
  size?: number;
  /** False shows the static logo, as with reduced motion. */
  motion?: boolean;
  label?: string;
  className?: string;
}

/** Looping Docklet loader for waits without a known end; static logo when motion is off or reduced. */
export function BrandLoader({ size = 48, motion = true, label = "Docklet Next", className }: BrandLoaderProps) {
  const id = useId().replace(/[^a-zA-Z0-9-]/g, "");
  const host = useRef<HTMLSpanElement>(null);
  const bar = useRef<HTMLElement>(null);
  const reduced = useReducedMotion();
  const animated = motion && !reduced;
  const markup = useMemo(
    () => ({ __html: renderLogoSVG({ candidate: FAVORITE_LOGO.candidate, variant: FAVORITE_LOGO.variant, tone: FAVORITE_LOGO.tone, motionStyle: FAVORITE_LOGO.motionStyle, animatable: true, idPrefix: `loader-${id}`, label, decorative: true }) }),
    [id, label],
  );

  useEffect(() => {
    const svg = host.current?.querySelector("svg");
    if (!animated || !svg || typeof svg.animate !== "function") return;
    const bowl = svg.querySelector<SVGPathElement>('[data-part="bowl"]');
    const glint = bowl ? (bowl.cloneNode(false) as SVGPathElement) : null;
    if (glint && bowl) {
      glint.removeAttribute("data-part");
      glint.setAttribute("data-loader-glint", "");
      glint.setAttribute("stroke-dasharray", ".3 2");
      glint.setAttribute("stroke-dashoffset", ".3");
      glint.setAttribute("opacity", "0");
      glint.style.stroke = "var(--dln-c-next)";
      bowl.after(glint);
    }
    let running: Animation[] = [];
    const cycle = () => { running = playCycle(svg, glint, bar.current); };
    cycle();
    const timer = setInterval(() => { running.forEach((a) => a.cancel()); cycle(); }, LOADER_CYCLE_MS);
    return () => {
      clearInterval(timer);
      running.forEach((a) => a.cancel());
      glint?.remove();
    };
  }, [animated, markup]);

  return (
    <span role="img" aria-label={label} data-slot="brand-loader" data-animated={animated || undefined} className={cn("inline-flex flex-col items-center gap-4", className)}>
      <span ref={host} aria-hidden data-slot="brand-logo" className="brand-logo" style={{ "--brand-logo-size": `${size}px` } as React.CSSProperties} dangerouslySetInnerHTML={markup} />
      {animated && (
        <span aria-hidden className="relative block h-0.5 w-40 overflow-hidden rounded-full bg-muted">
          <i ref={bar} className="brand-loader-bar absolute inset-0 w-2/5 -translate-x-full" />
        </span>
      )}
    </span>
  );
}
