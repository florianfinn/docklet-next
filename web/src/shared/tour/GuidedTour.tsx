// Source: docklet-next/issues/640
// Revision: 35f39e8729df29cabccc10a4adc42b4b68220a7c
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original guided tour: setup offer, navigation pointer and page spotlight.

import { useEffect, useId, useLayoutEffect, useRef, useState, useSyncExternalStore, type CSSProperties } from "react";
import { createPortal } from "react-dom";
import { useLocation, useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { identityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { useMotionEnabled } from "@/shared/theme/use-design-tokens";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { useSidebar } from "@/shared/ui/sidebar";

import { PAD, placeCard } from "./place-card";
import { TOUR_STEPS, type TourStep } from "./tour-steps";
import { endTour, goToStep, startTour, tourStore } from "./tour-store";

/** Lazy pages may take a moment; after this the card shows centred without a highlight. */
const ANCHOR_WAIT_MS = 1500;

/** The open phone navigation; the tour renders inside it so the sheet's focus scope keeps the card reachable. */
const OPEN_SHEET = '[data-slot="sidebar"][data-mobile="true"][data-state="open"]';
/** Another dialog (a discard confirmation, the search palette, a side sheet) takes precedence; the tour waits behind it. */
const OTHER_DIALOG = ['[role="alertdialog"]', '[data-slot="dialog-content"]', '[data-slot="sheet-content"]'].map((s) => `${s}[data-state="open"]`).join(", ");

const subscribeDom = (notify: () => void) => {
  const observer = new MutationObserver(notify);
  observer.observe(document.body, { childList: true, subtree: true, attributes: true, attributeFilter: ["data-state"] });
  return () => observer.disconnect();
};
const useElement = (selector: string) => useSyncExternalStore(subscribeDom, () => document.querySelector<HTMLElement>(selector));
/** A dialog still playing its exit animation; focus decisions wait until it is gone. */
const CLOSING_DIALOG = '[role="dialog"][data-state="closed"], [role="alertdialog"][data-state="closed"]';
const usable = (el: HTMLElement | null): el is HTMLElement => el !== null && el.isConnected && !el.closest('[aria-hidden="true"]');

/**
 * Returns focus once the tour has ended and any dialog closing with it (the phone menu on Escape)
 * is gone: into a modal that stays open, otherwise to the opener, otherwise to the page content.
 */
function returnFocus(opener: HTMLElement | null) {
  let clear = 0;
  const deadline = performance.now() + 1000;
  const settle = () => {
    if (tourStore.get().status === "running") return;
    clear = document.querySelector(CLOSING_DIALOG) && performance.now() < deadline ? 0 : clear + 1;
    // Two quiet frames let the closing dialog's own focus restoration run first.
    if (clear < 2) return void requestAnimationFrame(settle);
    const modal = document.querySelector<HTMLElement>(`${OPEN_SHEET}, ${OTHER_DIALOG}`);
    if (modal) {
      if (!modal.contains(document.activeElement)) modal.focus({ preventScroll: true });
      return;
    }
    (usable(opener) ? opener : document.getElementById("main-content"))?.focus({ preventScroll: true });
  };
  requestAnimationFrame(settle);
}

const subscribeResize = (notify: () => void) => {
  addEventListener("resize", notify);
  return () => removeEventListener("resize", notify);
};

export function GuidedTour() {
  const state = useStore(tourStore, (s) => s);
  const adapter = useAdapter();
  // A tour belongs to the signed-in administrator; a session change ends it.
  useEffect(() => identityLifetime(adapter).listen(endTour), [adapter]);
  if (state.status === "offer") return <TourOffer />;
  if (state.status === "running") return <TourRun index={state.index} />;
  return null;
}

function TourOffer() {
  const t = useTranslations("tour.offer");
  return (
    <Dialog open onOpenChange={(open) => !open && endTour()}>
      <DialogContent className="sm:max-w-md" data-tour="offer">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("text")}</DialogDescription>
        </DialogHeader>
        <p className="text-xs text-muted-foreground">{t("hint")}</p>
        <DialogFooter>
          <Button variant="outline" onClick={endTour}>{t("later")}</Button>
          <Button onClick={startTour} autoFocus>{t("start")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

interface Anchor {
  rect: DOMRect | null;
  el: HTMLElement | null;
  /** False while the page loads and the anchor is still searched. */
  settled: boolean;
}

/** "page" explains the step on its page; "nav" points at the navigation entry that opens it. */
type Phase = "page" | "nav";

const visible = (el: Element) => {
  const r = el.getBoundingClientRect();
  return r.width > 0 && r.height > 0;
};

/** Navigation entry of a route, or the menu button while the mobile navigation is closed. */
const navAnchors = (route: string) => [`a[data-slot="sidebar-menu-button"][href="${route}"]`, '[data-slot="sidebar-trigger"]'];

/** Tracks the first visible anchor of a step through scrolling, resizing and re-rendering. */
function useAnchor(step: TourStep, phase: Phase, motion: boolean): Anchor {
  const [anchor, setAnchor] = useState<Anchor & { for: string | null }>({ rect: null, el: null, settled: false, for: null });
  const target = `${step.key}:${phase}`;
  useEffect(() => {
    const candidates = phase === "nav" && step.route ? navAnchors(step.route) : step.anchors;
    const find = () => {
      for (const [rank, selector] of candidates.entries()) {
        const found = document.querySelector<HTMLElement>(selector);
        if (found && visible(found)) return { found, rank };
      }
      return null;
    };
    const observer = typeof ResizeObserver === "undefined" ? null : new ResizeObserver(() => measure());
    // The tracked anchor and the index of its candidate; a lower rank is a better anchor.
    let el: HTMLElement | null = null;
    let rank = Infinity;
    let frame = 0;
    const started = performance.now();
    const attach = (next: { found: HTMLElement; rank: number }) => {
      if (el) observer?.unobserve(el);
      el = next.found;
      rank = next.rank;
      observer?.observe(el);
      el.scrollIntoView?.({ block: phase === "nav" ? "nearest" : "center", behavior: motion ? "smooth" : "auto" });
    };
    const measure = () => {
      const held = el !== null && el.isConnected && visible(el);
      // A fallback gives way to a better anchor that renders later (a lazy page, the opened phone menu); a lost anchor to any remaining one.
      const best = rank > 0 || !held ? find() : null;
      if (best && (!held || best.rank < rank)) attach(best);
      else if (!held) {
        // Without any candidate the highlight clears and the card centres, once the initial wait is over.
        if (el) observer?.unobserve(el);
        if (!el && performance.now() - started <= ANCHOR_WAIT_MS) return;
        el = null;
        rank = Infinity;
        setAnchor((prev) => (prev.for === target && prev.settled && !prev.rect ? prev : { rect: null, el: null, settled: true, for: target }));
        return;
      }
      if (el) setAnchor({ rect: el.getBoundingClientRect(), el, settled: true, for: target });
    };
    const search = () => {
      const found = find();
      if (found) {
        attach(found);
        measure();
      } else if (performance.now() - started > ANCHOR_WAIT_MS) {
        setAnchor({ rect: null, el: null, settled: true, for: target });
      } else {
        frame = requestAnimationFrame(search);
      }
    };
    frame = requestAnimationFrame(search);
    // Pages replace their content after loading; a slow poll re-attaches a replaced anchor.
    const poll = setInterval(measure, 400);
    addEventListener("resize", measure);
    addEventListener("scroll", measure, true);
    return () => {
      cancelAnimationFrame(frame);
      clearInterval(poll);
      observer?.disconnect();
      removeEventListener("resize", measure);
      removeEventListener("scroll", measure, true);
    };
  }, [step, phase, target, motion]);
  // A result of another step or phase does not count.
  return anchor.for === target ? anchor : { rect: null, el: null, settled: false };
}

/** Click shields around the highlight; the hole stays open only while the user should click into it. */
function Shields({ rect, open }: { rect: DOMRect | null; open: boolean }) {
  if (!rect) return <div className="pointer-events-auto fixed inset-0 bg-black/70" />;
  // The hole matches the target exactly; the visual padding would expose neighbouring navigation entries.
  const top = Math.max(0, rect.top), left = Math.max(0, rect.left);
  const bottom = rect.bottom, right = rect.right;
  const parts: CSSProperties[] = [
    { top: 0, left: 0, right: 0, height: top },
    { top: bottom, left: 0, right: 0, bottom: 0 },
    { top, height: bottom - top, left: 0, width: left },
    { top, height: bottom - top, left: right, right: 0 },
  ];
  if (!open) parts.push({ top, left, width: right - left, height: bottom - top });
  return parts.map((style, i) => <div key={i} data-slot="tour-shield" className="pointer-events-auto fixed" style={style} />);
}

function TourRun({ index }: { index: number }) {
  const t = useTranslations("tour");
  const nav = useTranslations("shell.nav");
  const step = TOUR_STEPS[index] ?? TOUR_STEPS[0];
  const total = TOUR_STEPS.length;
  const last = index === total - 1;
  const { pathname } = useLocation();
  const navigate = useNavigate();
  const motion = useMotionEnabled();
  const { isMobile, openMobile } = useSidebar();
  const sheet = useElement(OPEN_SHEET);
  const waiting = useElement(OTHER_DIALOG) !== null;
  useSyncExternalStore(subscribeResize, () => `${innerWidth}x${innerHeight}`);
  // On another page the tour points at the navigation; the step continues once that page opens, by the link or Next.
  const phase: Phase = step.route && pathname !== step.route ? "nav" : "page";
  const { rect, el: anchorEl, settled } = useAnchor(step, phase, motion);
  const pointing = phase === "nav";
  const titleId = useId(), textId = useId();

  const card = useRef<HTMLElement>(null);
  const next = useRef<HTMLButtonElement>(null);
  const [size, setSize] = useState({ width: 340, height: 180 });
  useLayoutEffect(() => {
    const el = card.current;
    if (el && (el.offsetWidth !== size.width || el.offsetHeight !== size.height) && el.offsetWidth > 0) setSize({ width: el.offsetWidth, height: el.offsetHeight });
  }, [index, phase, settled, sheet, waiting, size.width, size.height]);
  useEffect(() => {
    const el = card.current;
    if (!el || typeof ResizeObserver === "undefined") return;
    const observer = new ResizeObserver(() => el.offsetWidth > 0 && setSize((s) => (s.width === el.offsetWidth && s.height === el.offsetHeight ? s : { width: el.offsetWidth, height: el.offsetHeight })));
    observer.observe(el);
    return () => observer.disconnect();
  }, [sheet, waiting]);
  useEffect(() => {
    if (settled && !waiting) next.current?.focus({ preventScroll: true });
  }, [index, pointing, settled, sheet, waiting]);
  const advance = () => {
    if (pointing && step.route) void navigate(step.route);
    else if (last) endTour();
    else goToStep(index + 1);
  };
  // Keyboard reach outside the card (the highlighted navigation entry while pointing) and the arrow-key steps.
  const reach = useRef<{ target: HTMLElement | null; forward: (() => void) | null; back: () => void }>({ target: null, forward: null, back: () => {} });
  useLayoutEffect(() => {
    reach.current = { target: pointing ? anchorEl : null, forward: last && !pointing ? null : advance, back: () => goToStep(index - 1) };
  });
  useEffect(() => {
    // Read live, not from render state: another dialog leads from the moment it opens until it is gone.
    const other = () => document.querySelector(OTHER_DIALOG) !== null;
    // Capture on window runs before the dialogs' own Escape handling, while they still count as open.
    const onKey = (e: globalThis.KeyboardEvent) => {
      const { target, forward, back } = reach.current;
      if (other() || !card.current) return;
      if (e.key === "Escape") endTour();
      // Arrow keys step from the card and from the highlighted entry alike.
      const active = document.activeElement;
      const owned = active instanceof Node && (card.current.contains(active) || target?.contains(active) === true);
      if (owned && e.key === "ArrowRight") forward?.();
      if (owned && e.key === "ArrowLeft") back();
      if (e.key !== "Tab") return;
      // The tour owns Tab: it cycles through the card's buttons and, while pointing, the highlighted entry.
      const items = [...card.current.querySelectorAll<HTMLElement>("button:not(:disabled)"), ...(target ? [target] : [])];
      const at = items.indexOf(document.activeElement as HTMLElement);
      e.preventDefault();
      e.stopPropagation();
      items[at < 0 ? (e.shiftKey ? items.length - 1 : 0) : (at + (e.shiftKey ? -1 : 1) + items.length) % items.length]?.focus();
    };
    // Focus that still lands behind the shields returns to the card.
    const onFocus = (e: FocusEvent) => {
      const node = e.target instanceof Node ? e.target : null;
      if (node && !other() && !card.current?.contains(node) && !reach.current.target?.contains(node)) next.current?.focus({ preventScroll: true });
    };
    // Focus lost with its element (a viewport change swaps the navigation, a page re-renders) returns as well.
    const lost = setInterval(() => {
      if (!other() && (document.activeElement === null || document.activeElement === document.body)) next.current?.focus({ preventScroll: true });
    }, 400);
    addEventListener("keydown", onKey, true);
    addEventListener("focusin", onFocus, true);
    const opener = document.activeElement instanceof HTMLElement && document.activeElement !== document.body ? document.activeElement : null;
    return () => {
      clearInterval(lost);
      removeEventListener("keydown", onKey, true);
      removeEventListener("focusin", onFocus, true);
      returnFocus(opener);
    };
  }, []);
  const position = placeCard(rect, size.width, size.height, innerWidth, innerHeight);
  const move = motion ? "transition-[top,left,width,height] duration-300 ease-out" : undefined;

  const spotlight: CSSProperties | undefined = rect
    ? { top: rect.top - PAD, left: rect.left - PAD, width: rect.width + PAD * 2, height: rect.height + PAD * 2, boxShadow: "0 0 0 9999px rgb(0 0 0 / 0.7)" }
    : undefined;
  const page = step.nav ? nav(step.nav) : "";
  const text = pointing ? t(isMobile && !openMobile ? "openMenu" : "openNav", { page }) : t(`steps.${step.key}.text`);

  if (waiting) return null;
  // Above the mobile navigation sheet (z-50), so the highlight stays visible while it is open.
  return createPortal(
    <div data-slot="tour-overlay" data-phase={phase} className="pointer-events-none fixed inset-0 z-[60]">
      {spotlight && (
        <div aria-hidden data-slot="tour-spotlight" className={cn("fixed rounded-xl", move)} style={spotlight}>
          {/* Only the frame pulses; the dimmed surroundings stay steady. */}
          <div className={cn("absolute inset-0 rounded-xl outline-2 outline-primary", pointing && motion && "animate-pulse")} />
        </div>
      )}
      <Shields rect={rect} open={pointing} />
      <section
        ref={card}
        role="dialog"
        aria-modal={!pointing}
        aria-labelledby={titleId}
        aria-describedby={textId}
        tabIndex={-1}
        data-slot="tour-card"
        data-step={step.key}
        className={cn("pointer-events-auto fixed flex max-h-[calc(100dvh-2rem)] w-[min(22rem,calc(100vw-2rem))] flex-col overflow-y-auto gap-3 rounded-xl border bg-popover p-4 text-popover-foreground shadow-xl outline-none", move, !settled && "opacity-0")}
        style={position}
      >
        <p className="text-xs text-muted-foreground tabular-nums">{t("progress", { current: index + 1, total })}</p>
        <div className="flex flex-col gap-1">
          <h2 id={titleId} className="text-base font-semibold">{t(`steps.${step.key}.title`)}</h2>
          <p id={textId} aria-live="polite" className="text-sm leading-relaxed text-muted-foreground">{text}</p>
        </div>
        <div className="flex gap-1" aria-hidden>
          {TOUR_STEPS.map((s, i) => (
            <span key={s.key} className={cn("h-1 flex-1 rounded-full", i < index || (i === index && !pointing) ? "bg-primary" : "bg-muted")} />
          ))}
        </div>
        <div className="flex items-center justify-between gap-2">
          <Button variant="ghost" size="sm" onClick={endTour}>{t("skip")}</Button>
          <div className="flex gap-2">
            {index > 0 && <Button variant="outline" size="sm" onClick={() => goToStep(index - 1)}>{t("back")}</Button>}
            <Button ref={next} size="sm" onClick={advance}>{last && !pointing ? t("finish") : t("next")}</Button>
          </div>
        </div>
      </section>
    </div>,
    sheet ?? document.body,
  );
}
