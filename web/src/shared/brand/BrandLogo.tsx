import { useEffect, useId, useLayoutEffect, useMemo, useRef } from "react";

import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";

import { playedSlots as played, replayStore } from "./brand-entrance";
import "./brand-logo.css";
import { playEntrance, type Entrance } from "./play-entrance";
import { FAVORITE_LOGO, renderLogoSVG, type LogoMotionStyle, type LogoVariant } from "./vendor/logo-svg.js";

interface BrandLogoProps {
  variant?: LogoVariant;
  motionStyle?: LogoMotionStyle;
  /** Height in px. */
  size?: number;
  /** Plays the entrance once per page load for this slot; omit for a static logo. */
  entranceSlot?: string;
  /** False when the user or the lab turned motion off; a running entrance then jumps to its end. */
  motion?: boolean;
  label?: string;
  decorative?: boolean;
  className?: string;
}

/**
 * The Docklet Next logo, rendered from the vendored study renderer as inline
 * SVG (no image, no iframe). Colours come from the palette roles through
 * brand-logo.css; the entrance runs on WAAPI and leaves the static SVG.
 */
export function BrandLogo({ variant = FAVORITE_LOGO.variant, motionStyle = FAVORITE_LOGO.motionStyle, size = 32, entranceSlot, motion = true, label = "Docklet Next", decorative = false, className }: BrandLogoProps) {
  const id = useId().replace(/[^a-zA-Z0-9-]/g, "");
  const host = useRef<HTMLSpanElement>(null);
  const running = useRef<Entrance | null>(null);
  const replay = useStore(replayStore, (n) => n);
  const markup = useMemo(
    () => renderLogoSVG({ candidate: FAVORITE_LOGO.candidate, variant, tone: FAVORITE_LOGO.tone, motionStyle, animatable: true, idPrefix: `brand-${id}`, label, decorative }),
    [variant, motionStyle, id, label, decorative],
  );
  const html = useMemo(() => ({ __html: markup }), [markup]);

  const play = () => {
    const svg = host.current?.querySelector("svg");
    running.current?.animations.forEach((a) => a.cancel());
    running.current = svg ? playEntrance(svg, motionStyle) : null;
  };

  // First mount of a slot plays once; later mounts and markup changes show the settled logo.
  // No cancel on unmount: a replaced SVG takes its animations along, and StrictMode's
  // effect re-run keeps the same SVG, so the entrance continues.
  useLayoutEffect(() => {
    if (!entranceSlot || !motion || played.has(entranceSlot)) return;
    played.add(entranceSlot);
    play();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- plays on the first render of the slot only
  }, [entranceSlot]);

  useLayoutEffect(() => {
    if (replay > 0 && entranceSlot && motion) play();
    // eslint-disable-next-line react-hooks/exhaustive-deps -- explicit replay requests only
  }, [replay]);

  useEffect(() => {
    if (!motion) running.current?.animations.forEach((a) => a.finish());
  }, [motion]);

  return <span ref={host} className={cn("brand-logo", className)} style={{ "--brand-logo-size": `${size}px` } as React.CSSProperties} data-slot="brand-logo" data-variant={variant} dangerouslySetInnerHTML={html} />;
}
