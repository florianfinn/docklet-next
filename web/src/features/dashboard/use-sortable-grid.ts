import { useCallback, useEffect, useLayoutEffect, useRef, useState, type PointerEvent as ReactPointerEvent } from "react";

const START_DISTANCE = 5;
const INTERACTIVE = "button, a, input, select, textarea, [role='button'], [role='switch'], [role='checkbox']";
const EASING = "cubic-bezier(0.22, 1, 0.36, 1)";

interface Options<T extends string> {
  items: T[];
  enabled: boolean;
  animate: boolean;
  /** Animation duration in ms. */
  duration: number;
  onCommit: (order: T[]) => void;
}

interface Drag<T> {
  id: T;
  pointerId: number;
  startX: number;
  startY: number;
  started: boolean;
  /** Slot position when the drag started; the card is translated relative to it. */
  origin: DOMRect | null;
  before: T[];
}

/** Pure part: the order after dropping `dragged` before or after `target`. */
export function insertAt<T>(order: readonly T[], dragged: T, target: T, after: boolean): T[] {
  if (dragged === target) return [...order];
  const rest = order.filter((x) => x !== dragged);
  const at = rest.indexOf(target) + (after ? 1 : 0);
  rest.splice(at, 0, dragged);
  return rest;
}

const rects = <T,>(map: Map<T, HTMLElement>) => new Map([...map].map(([k, el]) => [k, el.getBoundingClientRect()]));

/**
 * Pointer drag and drop for a CSS grid of equal cards. The dragged card follows
 * the pointer via transform (no React render per move); the others reorder
 * live and slide into place (FLIP with the Web Animations API). Mouse, pen and
 * touch share the pointer events; Escape cancels and restores the order.
 */
export function useSortableGrid<T extends string>({ items, enabled, animate, duration, onCommit }: Options<T>) {
  const [preview, setPreview] = useState<T[] | null>(null);
  const [dragging, setDragging] = useState<T | null>(null);
  const slots = useRef(new Map<T, HTMLElement>());
  const cards = useRef(new Map<T, HTMLElement>());
  const drag = useRef<Drag<T> | null>(null);
  const flipFrom = useRef<Map<T, DOMRect> | null>(null);
  const pointer = useRef({ x: 0, y: 0 });
  const detach = useRef<(() => void) | null>(null);
  const order = preview ?? items;
  const orderRef = useRef(order);
  const opts = useRef({ animate, duration, onCommit });

  useLayoutEffect(() => {
    orderRef.current = order;
    opts.current = { animate, duration, onCommit };
  });

  const placeCard = useCallback(() => {
    const d = drag.current;
    if (!d?.started || !d.origin) return;
    const card = cards.current.get(d.id);
    const slot = slots.current.get(d.id);
    if (!card || !slot) return;
    const now = slot.getBoundingClientRect();
    const dx = pointer.current.x - d.startX - (now.left - d.origin.left);
    const dy = pointer.current.y - d.startY - (now.top - d.origin.top);
    card.style.transform = `translate(${dx}px, ${dy}px) scale(1.02)`;
  }, []);

  // After a live reorder: slide the moved siblings from their old spot and keep the card under the pointer.
  useLayoutEffect(() => {
    const from = flipFrom.current;
    flipFrom.current = null;
    if (from && opts.current.animate) {
      for (const [id, el] of slots.current) {
        if (id === drag.current?.id) continue;
        const a = from.get(id);
        if (!a) continue;
        const b = el.getBoundingClientRect();
        const dx = a.left - b.left;
        const dy = a.top - b.top;
        if (dx || dy) el.animate([{ transform: `translate(${dx}px, ${dy}px)` }, { transform: "none" }], { duration: opts.current.duration, easing: EASING });
      }
    }
    placeCard();
  }, [preview, placeCard]);

  const finish = useCallback((commit: boolean) => {
    detach.current?.();
    detach.current = null;
    const d = drag.current;
    drag.current = null;
    if (!d) return;
    const card = cards.current.get(d.id);
    if (card) {
      const from = card.style.transform;
      card.style.transform = "";
      if (opts.current.animate && from && d.started) card.animate([{ transform: from }, { transform: "none" }], { duration: opts.current.duration, easing: EASING });
    }
    if (d.started && commit) opts.current.onCommit(orderRef.current);
    if (d.started && !commit) {
      flipFrom.current = rects(slots.current);
      setPreview(null);
    } else setPreview(null);
    setDragging(null);
  }, []);

  const onMove = useCallback(
    (e: PointerEvent) => {
      const d = drag.current;
      if (!d || e.pointerId !== d.pointerId) return;
      pointer.current = { x: e.clientX, y: e.clientY };
      if (!d.started) {
        if (Math.hypot(e.clientX - d.startX, e.clientY - d.startY) < START_DISTANCE) return;
        d.started = true;
        d.origin = slots.current.get(d.id)?.getBoundingClientRect() ?? null;
        setDragging(d.id);
      }
      placeCard();
      for (const [id, el] of slots.current) {
        if (id === d.id) continue;
        const r = el.getBoundingClientRect();
        if (e.clientX < r.left || e.clientX > r.right || e.clientY < r.top || e.clientY > r.bottom) continue;
        const next = insertAt(orderRef.current, d.id, id, e.clientX > r.left + r.width / 2);
        if (next.join() !== orderRef.current.join()) {
          flipFrom.current = rects(slots.current);
          orderRef.current = next;
          setPreview(next);
        }
        break;
      }
    },
    [placeCard],
  );

  const onPointerDown = useCallback(
    (id: T, e: ReactPointerEvent) => {
      if (!enabled || e.button !== 0 || drag.current) return;
      const target = e.target as HTMLElement;
      if (target.closest(INTERACTIVE) && !target.closest("[data-drag-handle]")) return;
      e.preventDefault();
      drag.current = { id, pointerId: e.pointerId, startX: e.clientX, startY: e.clientY, started: false, origin: null, before: orderRef.current };
      pointer.current = { x: e.clientX, y: e.clientY };
      const up = (ev: PointerEvent) => ev.pointerId === drag.current?.pointerId && finish(ev.type === "pointerup");
      const key = (ev: KeyboardEvent) => {
        if (ev.key === "Escape" && drag.current?.started) {
          ev.preventDefault();
          finish(false);
        }
      };
      window.addEventListener("pointermove", onMove);
      window.addEventListener("pointerup", up);
      window.addEventListener("pointercancel", up);
      window.addEventListener("keydown", key);
      detach.current = () => {
        window.removeEventListener("pointermove", onMove);
        window.removeEventListener("pointerup", up);
        window.removeEventListener("pointercancel", up);
        window.removeEventListener("keydown", key);
      };
    },
    [enabled, finish, onMove],
  );

  useEffect(() => () => detach.current?.(), []);

  const slotRef = useCallback(
    (id: T) => (el: HTMLElement | null) => {
      if (el) slots.current.set(id, el);
      else slots.current.delete(id);
    },
    [],
  );
  const cardRef = useCallback(
    (id: T) => (el: HTMLElement | null) => {
      if (el) cards.current.set(id, el);
      else cards.current.delete(id);
    },
    [],
  );

  return { order, dragging, onPointerDown, slotRef, cardRef };
}
