import { useCallback, useEffect, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";

import { clampSize, WIDGET_COLUMN_REM, WIDGET_GAP_REM, WIDGET_ROW_REM, type WidgetId, type WidgetSize } from "./dashboard-layout";

const remPx = () => (typeof document === "undefined" ? 16 : parseFloat(getComputedStyle(document.documentElement).fontSize) || 16);

/** Columns that fit a width: one per WIDGET_COLUMN_REM, at least one. */
export function columnsFor(widthPx: number, rem: number = 16): number {
  const col = WIDGET_COLUMN_REM * rem;
  const gap = WIDGET_GAP_REM * rem;
  return Math.max(1, Math.floor((widthPx + gap) / (col + gap)));
}

/** Grid placement of one widget; wider sizes than the current columns are capped, nothing overlaps. */
export function spanStyle(size: WidgetSize, columns: number): React.CSSProperties {
  return { gridColumn: `span ${Math.min(size.w, columns)}`, gridRow: `span ${size.h}` };
}

/**
 * Responsive widget grid: measures its width and derives the column count, so
 * widths snap to whole columns on every screen (one column on phones, several
 * on wide and ultrawide displays). Rows have a fixed height.
 */
export function useWidgetGrid() {
  const [el, setEl] = useState<HTMLDivElement | null>(null);
  const attach = useCallback((node: HTMLDivElement | null) => setEl(node), []);
  const [columns, setColumns] = useState(1);
  // The grid mounts after the skeleton, so measuring follows the element, not the hook's mount.
  useLayoutEffect(() => {
    if (!el) return;
    const measure = () => setColumns(columnsFor(el.clientWidth, remPx()));
    measure();
    if (typeof ResizeObserver === "undefined") return;
    const ro = new ResizeObserver(measure);
    ro.observe(el);
    return () => ro.disconnect();
  }, [el]);
  const style: React.CSSProperties = { gridTemplateColumns: `repeat(${columns}, minmax(0, 1fr))`, gridAutoRows: `${WIDGET_ROW_REM}rem`, gap: `${WIDGET_GAP_REM}rem` };
  return { attach, el, columns, style };
}

/** Pure part of a pointer resize: start size plus pointer delta, snapped to whole cells and clamped. */
export function snapSize(id: WidgetId, start: WidgetSize, startPx: { w: number; h: number }, delta: { x: number; y: number }, cell: { col: number; row: number; gap: number }, columns: number): WidgetSize {
  const w = Math.round((startPx.w + delta.x + cell.gap) / (cell.col + cell.gap));
  const h = Math.round((startPx.h + delta.y + cell.gap) / (cell.row + cell.gap));
  return clampSize(id, { w: Math.max(1, w), h: Math.max(1, h) }, columns || start.w);
}

interface ResizeOptions {
  columns: number;
  grid: HTMLDivElement | null;
  onPreview: (id: WidgetId, size: WidgetSize | null) => void;
  onCommit: (id: WidgetId, size: WidgetSize) => void;
}

/**
 * Corner handle drag: the preview size follows the pointer in whole cells (a
 * render only when the snapped size changes), release commits, Escape cancels.
 * The handle is a button, so the card's drag-to-move ignores it.
 */
export function useWidgetResize({ columns, grid, onPreview, onCommit }: ResizeOptions) {
  const detach = useRef<(() => void) | null>(null);
  const opts = useRef({ columns, grid, onPreview, onCommit });
  useLayoutEffect(() => {
    opts.current = { columns, grid, onPreview, onCommit };
  });
  useEffect(() => () => detach.current?.(), []);

  return useCallback(
    (id: WidgetId, start: WidgetSize, e: ReactPointerEvent<HTMLElement>) => {
      if (e.button !== 0 || detach.current) return;
      const slot = e.currentTarget.closest<HTMLElement>("[data-widget-slot]");
      const grid = opts.current.grid;
      if (!slot || !grid) return;
      e.preventDefault();
      e.stopPropagation();
      const rem = remPx();
      const gap = WIDGET_GAP_REM * rem;
      const cols = opts.current.columns;
      const cell = { col: (grid.clientWidth - gap * (cols - 1)) / cols, row: WIDGET_ROW_REM * rem, gap };
      const rect = slot.getBoundingClientRect();
      const origin = { x: e.clientX, y: e.clientY };
      const pointerId = e.pointerId;
      let last = start;
      const move = (ev: PointerEvent) => {
        if (ev.pointerId !== pointerId) return;
        const next = snapSize(id, start, { w: rect.width, h: rect.height }, { x: ev.clientX - origin.x, y: ev.clientY - origin.y }, cell, cols);
        if (next.w !== last.w || next.h !== last.h) {
          last = next;
          opts.current.onPreview(id, next);
        }
      };
      const end = (commit: boolean) => {
        detach.current?.();
        detach.current = null;
        opts.current.onPreview(id, null);
        if (commit && (last.w !== start.w || last.h !== start.h)) opts.current.onCommit(id, last);
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
    },
    [],
  );
}
