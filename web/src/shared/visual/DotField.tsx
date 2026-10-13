import { useEffect, useRef } from "react";

import { cn } from "@/shared/lib/utils";

import { attachDotField } from "./dot-field";

/**
 * Animated point surface behind a `.dot-grid` element. Put it first inside
 * the surface; it takes no pointer events and is hidden from assistive
 * technology. `fixed` covers the viewport instead of the element, for
 * full-screen pages that scroll (setup, login), so the canvas never grows
 * with the page. While it runs, the static CSS dots of `.dot-grid` are off.
 */
export function DotField({ fixed, className }: { fixed?: boolean; className?: string }) {
  const ref = useRef<HTMLCanvasElement>(null);
  useEffect(() => {
    const canvas = ref.current;
    return canvas ? attachDotField(canvas) : undefined;
  }, []);
  return <canvas ref={ref} aria-hidden data-slot="dot-field" className={cn("pointer-events-none -z-10 block size-full", fixed ? "fixed inset-0 h-svh w-screen" : "absolute inset-0", className)} />;
}
