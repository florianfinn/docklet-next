import { useCallback, useEffect, useRef, useState, type KeyboardEvent } from "react";
import { useNavigate } from "react-router";

import type { Host } from "@/domain/model";
import type { MenuGroups } from "@/shared/components/action-menu";
import { useExclusivePopup } from "@/shared/lib/popup-coordinator";

import type { FleetState, FleetUnit } from "./fleet-status";

/** Segment colours shared by ring and line: the fleet state's tone, never the palette. */
export const TONE_VAR: Record<string, string> = {
  ok: "var(--success)",
  err: "var(--destructive)",
  info: "var(--info)",
  warn: "var(--warning)",
  gray: "color-mix(in oklch, var(--muted-foreground) 45%, transparent)",
  orange: "color-mix(in oklch, var(--orange) 55%, transparent)",
};

/** Stack page, the single container of a group, or the container list filtered to the group's origin. */
export function hrefOf(u: FleetUnit): string {
  if (u.stackId) return `/stacks/${u.stackId}`;
  if (u.containers.length === 1 && u.containers[0]) return `/containers?c=${u.containers[0].id}`;
  return `/containers?origin=${u.origin ?? "standalone"}`;
}

/** Menu entries that only navigate or change the overview; everything else sends a command to the host. */
const NAVIGATION = new Set(["open", "edit", "logs", "details", "stack", "image", "hide", "restore", "list", "member"]);

/** Commands to a disconnected host cannot run; navigation stays available. */
export function guardUnreachable(groups: MenuGroups, unreachable: boolean): MenuGroups {
  if (!unreachable) return groups;
  return groups.map((group) => group.map((a) => (NAVIGATION.has(a.id.split(":")[0] ?? a.id) ? a : { ...a, disabled: true })));
}

/** Space between a ring and its info card, and the card's distance to the viewport edge. */
const GAP = 12;
const EDGE = 8;

export interface InfoPlacement {
  left: number;
  top: number;
  side: "left" | "right" | "below" | "above";
  /** Set when the card only fits outside the ring shortened to this height. */
  maxHeight?: number;
}

/** Shortest card worth showing; below it the card may overlap the ring instead. */
const MIN_CARD = 120;

/**
 * Info card position: beside the ring box (never over it), on the side of the
 * active segment when it fits, at the segment's height; below or above the
 * ring when neither side fits, shortened if needed. `bound` is the left edge of the page content,
 * so the card never covers the navigation.
 */
export function placeInfoCard(ring: DOMRect, point: { x: number; y: number }, card: { width: number; height: number }, viewport: { width: number; height: number }, bound: number, preferLeft: boolean): InfoPlacement {
  const clampY = (y: number) => Math.min(Math.max(EDGE, y), Math.max(EDGE, viewport.height - card.height - EDGE));
  const fitsRight = ring.right + GAP + card.width <= viewport.width - EDGE;
  const fitsLeft = ring.left - GAP - card.width >= bound + EDGE;
  const top = clampY(point.y - card.height / 2);
  if (fitsLeft && (preferLeft || !fitsRight)) return { left: ring.left - GAP - card.width, top, side: "left" };
  if (fitsRight) return { left: ring.right + GAP, top, side: "right" };
  const left = Math.min(Math.max(EDGE, ring.left + ring.width / 2 - card.width / 2), Math.max(EDGE, viewport.width - card.width - EDGE));
  const spaceBelow = viewport.height - EDGE - ring.bottom - GAP;
  const spaceAbove = ring.top - GAP - EDGE;
  if (card.height <= spaceBelow) return { left, top: ring.bottom + GAP, side: "below" };
  if (card.height <= spaceAbove) return { left, top: ring.top - GAP - card.height, side: "above" };
  // Neither side holds the whole card (phones): shorten it on the roomier side rather than cover the ring.
  const below = spaceBelow >= spaceAbove;
  const height = Math.max(MIN_CARD, below ? spaceBelow : spaceAbove);
  const y = below ? Math.min(ring.bottom + GAP, viewport.height - EDGE - height) : Math.max(EDGE, ring.top - GAP - height);
  return { left, top: y, side: below ? "below" : "above", maxHeight: height };
}

const OPEN_DELAY = 120;
const CLOSE_DELAY = 180;

export interface FleetSegment {
  item: FleetUnit;
  host: Host;
  state: FleetState;
  index: number;
}

/**
 * One tab stop for all segments (roving tabindex), arrows/Home/End move, Enter
 * or Space opens the target. Hover (mouse only), focus and the arrow keys show
 * the popup with a short delay; it is exclusive with the other info popups and
 * stays open while the pointer is over it. Moving between segments keeps it.
 */
export function useSegmentPopup(segments: FleetSegment[], refs: { group: React.RefObject<Element | null>; content: React.RefObject<HTMLDivElement | null> }) {
  const navigate = useNavigate();
  const [active, setActive] = useState(0);
  const [focusIndex, setFocusIndex] = useState(0);
  const timer = useRef<ReturnType<typeof setTimeout>>(undefined);
  const { group: groupRef, content: contentRef } = refs;
  const segRefs = useRef<(HTMLElement | SVGElement | null)[]>([]);
  const [open, setOpen] = useExclusivePopup(() => clearTimeout(timer.current));
  useEffect(() => () => clearTimeout(timer.current), []);

  const show = useCallback(
    (index: number, delay: number) => {
      clearTimeout(timer.current);
      setActive(index);
      if (delay) timer.current = setTimeout(() => setOpen(true), delay);
      else setOpen(true);
    },
    [setOpen],
  );
  const hide = useCallback(
    (delay = CLOSE_DELAY) => {
      clearTimeout(timer.current);
      timer.current = setTimeout(() => setOpen(false), delay);
    },
    [setOpen],
  );

  const onKeyDown = (e: KeyboardEvent) => {
    const n = segments.length;
    const step = { ArrowRight: 1, ArrowDown: 1, ArrowLeft: -1, ArrowUp: -1 }[e.key];
    let next: number | undefined;
    if (step) next = (focusIndex + step + n) % n;
    else if (e.key === "Home") next = 0;
    else if (e.key === "End") next = n - 1;
    if (next !== undefined) {
      e.preventDefault();
      setFocusIndex(next);
      (segRefs.current[next] as HTMLElement | null)?.focus();
      return;
    }
    const seg = segments[focusIndex];
    if ((e.key === "Enter" || e.key === " ") && seg) {
      e.preventDefault();
      navigate(hrefOf(seg.item));
    } else if (e.key === "Escape" && open) {
      e.stopPropagation();
      setOpen(false);
    }
  };

  const segmentProps = (index: number) => ({
    ref: (el: HTMLElement | SVGElement | null) => {
      segRefs.current[index] = el;
    },
    tabIndex: index === focusIndex ? 0 : -1,
    onClick: () => segments[index] && navigate(hrefOf(segments[index].item)),
    onPointerEnter: (ev: React.PointerEvent) => ev.pointerType === "mouse" && show(index, open ? 0 : OPEN_DELAY),
    onFocus: () => {
      setFocusIndex(index);
      show(index, 0);
    },
    onBlur: (ev: React.FocusEvent) => {
      if (!contentRef.current?.contains(ev.relatedTarget as Node | null)) hide(0);
    },
  });

  const current = segments[Math.min(active, segments.length - 1)];
  const contentProps = {
    "data-slot": "ring-popup",
    onOpenAutoFocus: (e: Event) => e.preventDefault(),
    onCloseAutoFocus: (e: Event) => e.preventDefault(),
    onPointerEnter: () => clearTimeout(timer.current),
    onPointerLeave: (e: React.PointerEvent) => e.pointerType === "mouse" && !contentRef.current?.contains(document.activeElement) && hide(),
    onEscapeKeyDown: () => (segRefs.current[focusIndex] as HTMLElement | null)?.focus(),
    // Moving between segments is not "outside": the popup follows instead of closing.
    onFocusOutside: (e: Event) => groupRef.current?.contains(e.target as Node) && e.preventDefault(),
  };

  return { open: open && !!current, setOpen, active, current, onKeyDown, hide, segmentProps, contentProps };
}
