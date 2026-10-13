import { useCallback, useEffect, useLayoutEffect, useRef, type PointerEvent as ReactPointerEvent } from "react";

import { WIDGET_GAP_REM, WIDGET_ROW_REM } from "./dashboard-layout";
import { clampHostArea, type HostAreaSize } from "./fleet-view-store";

const remPx = () => (typeof document === "undefined" ? 16 : parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);

/** Minimum height of the host area for `h` dashboard rows, in rem, matching the widget grid's rows. */
export function hostAreaMinHeightRem(h: number): number {
  return h * WIDGET_ROW_REM + (h - 1) * WIDGET_GAP_REM;
}

/**
 * Pure part of a pointer resize: width in twelfths of the summary, height in
 * dashboard rows. `widthFromLeft` inverts x for a handle on the left edge
 * (host area on the right of the visual), so dragging left widens it.
 */
export function snapHostArea(start: HostAreaSize, delta: { x: number; y: number }, cell: { twelfth: number; row: number }, widthFromLeft: boolean): HostAreaSize {
  const dx = widthFromLeft ? -delta.x : delta.x;
  return clampHostArea({ w: start.w + Math.round(dx / Math.max(1, cell.twelfth)), h: start.h + Math.round(delta.y / Math.max(1, cell.row)) });
}

interface Options {
  /** Width only counts when the area sits beside the visual. */
  resizableWidth: boolean;
  onPreview: (size: HostAreaSize | null) => void;
  onCommit: (size: HostAreaSize) => void;
}

/** Which size a handle changes: the corner both, the edge handles one each. */
export type ResizeAxis = "both" | "w" | "h";

/** Pointer drag of a host area handle: preview in whole cells, release commits, Escape cancels. */
export function useHostAreaResize({ resizableWidth, onPreview, onCommit }: Options) {
  const detach = useRef<(() => void) | null>(null);
  const opts = useRef({ resizableWidth, onPreview, onCommit });
  useLayoutEffect(() => {
    opts.current = { resizableWidth, onPreview, onCommit };
  });
  useEffect(() => () => detach.current?.(), []);

  return useCallback((start: HostAreaSize, e: ReactPointerEvent<HTMLElement>, axis: ResizeAxis = "both") => {
    if (e.button !== 0 || detach.current) return;
    const summary = e.currentTarget.closest<HTMLElement>("[data-slot='ops-summary']");
    if (!summary) return;
    e.preventDefault();
    e.stopPropagation();
    const rem = remPx();
    const cell = { twelfth: summary.clientWidth / 12, row: (WIDGET_ROW_REM + WIDGET_GAP_REM) * rem };
    const origin = { x: e.clientX, y: e.clientY };
    const pointerId = e.pointerId;
    let last = start;
    const move = (ev: PointerEvent) => {
      if (ev.pointerId !== pointerId) return;
      const raw = snapHostArea(start, { x: ev.clientX - origin.x, y: ev.clientY - origin.y }, cell, true);
      const next = { w: opts.current.resizableWidth && axis !== "h" ? raw.w : start.w, h: axis !== "w" ? raw.h : start.h };
      if (next.w !== last.w || next.h !== last.h) {
        last = next;
        opts.current.onPreview(next);
      }
    };
    const end = (commit: boolean) => {
      detach.current?.();
      detach.current = null;
      opts.current.onPreview(null);
      if (commit && (last.w !== start.w || last.h !== start.h)) opts.current.onCommit(last);
    };
    const up = (ev: PointerEvent) => ev.pointerId === pointerId && end(ev.type === "pointerup");
    const key = (ev: KeyboardEvent) => {
      if (ev.key !== "Escape") return;
      ev.preventDefault();
      end(false);
    };
    window.addEventListener("pointermove", move);
    window.addEventListener("pointerup", up);
    window.addEventListener("pointercancel", up);
    window.addEventListener("keydown", key);
    detach.current = () => {
      window.removeEventListener("pointermove", move);
      window.removeEventListener("pointerup", up);
      window.removeEventListener("pointercancel", up);
      window.removeEventListener("keydown", key);
    };
  }, []);
}
