import { useCallback, useEffect, useId, useLayoutEffect, useRef } from "react";
import { createPortal } from "react-dom";
import { useTranslations } from "use-intl";

import { setPreference, usePreference } from "@/shared/preferences/preferences-store";

import type { Host } from "@/domain/model";

import { placeInfoCard, TONE_VAR, useSegmentPopup, type FleetSegment } from "./fleet-segments";
import { FLEET_TONE, type FleetUnit } from "./fleet-status";
import { arcPath, glowRegion, hitBand, hitCells, layoutRing, polar, ringGeometry, sectorPath, segmentFan } from "./ring-geometry";
import { SegmentCard } from "./SegmentCard";
import { SegmentMenu } from "./SegmentMenu";
import { useUnitName } from "./unit-name";

const GLOW_BLUR = 3;
/** Disconnected host: one faint block with a dashed outline, neither confirmed running nor an error fill. */
const UNREACHABLE_FILL = "color-mix(in oklch, var(--destructive) 22%, transparent)";
interface FleetRingProps {
  hosts: Host[];
  /** Stacks and origin groups (see fleetUnits). */
  units: FleetUnit[];
  label: string;
  center: { value: string; caption: string };
  size?: number;
  /** Hovered or focused segment moves outward; the info card opens on that side when there is room. */
  fan?: boolean;
}

/**
 * Every stack (and each origin group of containers outside Compose) as one
 * ring segment, grouped by host with the host colour on the outer arc. Colour
 * = aggregated fleet state; a disconnected host's arc and its stacks turn red
 * and dashed.
 *
 * Hover, focus or the arrow keys show a read-only info card beside the ring:
 * it has no controls, takes no pointer events and never covers the ring, so
 * the pointer can move freely along the circle. Click or Enter opens the
 * target; right click, long press, the context-menu key or Shift+F10 opens the
 * segment's actions. With `fan`, the active segment moves radially outward.
 * Pointer input lands on a static hit cell per segment that covers both the
 * resting and the fanned position, so the target never slips away mid-move.
 */
export function FleetRing({ hosts, units, label, center, size = 220, fan = true }: FleetRingProps) {
  const t = useTranslations("dashboard.ring");
  const showUnraid = usePreference("showUnraidRing");
  const visibleUnits = showUnraid ? units : units.filter((unit) => unit.origin !== "unraid");
  const ts = useTranslations("common.status");
  const unitName = useUnitName();
  const uid = useId().replace(/:/g, "");
  const glowId = `ring-glow-${uid}`;
  const infoId = `ring-info-${uid}`;
  const g = ringGeometry(size);
  const c = size / 2;
  const groups = layoutRing(hosts.map((h) => ({ group: h, items: visibleUnits.filter((u) => u.hostId === h.id) })));
  const glow = glowRegion(size, GLOW_BLUR);
  const flat = groups.flatMap((gr) => gr.segments.map((s) => ({ ...s, host: gr.group })));
  const cells = hitCells(flat, (g.hitPad / g.segR) * (180 / Math.PI));
  const band = hitBand(g, fan);
  const segments: FleetSegment[] = flat.map((s) => ({ item: s.item, host: s.host, state: s.item.state, index: s.index }));
  const svgRef = useRef<SVGSVGElement>(null);
  const infoRef = useRef<HTMLDivElement>(null);
  const wrapRef = useRef<HTMLDivElement>(null);
  const segEls = useRef<(SVGPathElement | null)[]>([]);
  const keyboardMenu = useRef(false);
  const popup = useSegmentPopup(segments, { group: svgRef, content: infoRef });

  const currentSeg = flat[Math.min(popup.active, flat.length - 1)];
  const mid = currentSeg ? (currentSeg.start + currentSeg.end) / 2 : 90;
  const fanned = fan && popup.open;
  const leftHalf = Math.sin((mid * Math.PI) / 180) < -1e-6;
  const wide = currentSeg ? currentSeg.end - currentSeg.start > 180 : false;
  const outer = polar(c, c, g.segR + g.segW / 2 + (fanned && !wide ? g.fan : 0), mid);

  const place = useCallback(() => {
    const card = infoRef.current;
    const wrap = wrapRef.current;
    if (!card || !wrap) return;
    const box = wrap.getBoundingClientRect();
    const scale = box.width / size || 1;
    const bound = wrap.closest("main")?.getBoundingClientRect().left ?? 0;
    const body = card.firstElementChild as HTMLElement | null;
    if (body) body.style.maxHeight = "";
    const p = placeInfoCard(box, { x: box.left + outer.x * scale, y: box.top + outer.y * scale }, { width: card.offsetWidth, height: card.offsetHeight }, { width: window.innerWidth, height: window.innerHeight }, bound, leftHalf);
    if (body && p.maxHeight) {
      body.style.maxHeight = `${Math.floor(p.maxHeight)}px`;
      body.style.maskImage = "linear-gradient(to bottom, #000 calc(100% - 1.5rem), transparent)";
    } else if (body) body.style.maskImage = "";
    card.style.transform = `translate(${Math.round(p.left)}px, ${Math.round(p.top)}px)`;
    card.dataset.side = p.side;
    card.style.visibility = "visible";
  }, [outer.x, outer.y, size, leftHalf]);

  useLayoutEffect(() => {
    if (popup.open) place();
  }, [popup.open, popup.active, place]);

  useEffect(() => {
    if (!popup.open) return;
    window.addEventListener("scroll", place, { passive: true, capture: true });
    window.addEventListener("resize", place);
    return () => {
      window.removeEventListener("scroll", place, { capture: true });
      window.removeEventListener("resize", place);
    };
  }, [popup.open, place]);

  const onKeyDown = (e: React.KeyboardEvent<SVGSVGElement>) => {
    if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
      const el = segEls.current.find((s) => s === document.activeElement);
      if (!el) return;
      e.preventDefault();
      const r = el.getBoundingClientRect();
      keyboardMenu.current = true;
      el.dispatchEvent(new MouseEvent("contextmenu", { bubbles: true, cancelable: true, clientX: r.left + r.width / 2, clientY: r.top + r.height / 2 }));
      return;
    }
    popup.onKeyDown(e);
  };

  return (
    <div ref={wrapRef} className="relative shrink-0" style={{ width: size, height: size }} data-slot="fleet-ring" data-fan={fan || undefined}>
      <svg
        ref={svgRef}
        viewBox={`0 0 ${size} ${size}`}
        width={size}
        height={size}
        role="group"
        aria-label={label}
        aria-roledescription={t("roleDescription")}
        className="overflow-visible"
        onKeyDown={onKeyDown}
        onPointerLeave={(e) => e.pointerType === "mouse" && popup.hide()}
      >
        <defs>
          <filter id={glowId} filterUnits="userSpaceOnUse" x={glow.x} y={glow.y} width={glow.width} height={glow.height}>
            <feGaussianBlur stdDeviation={GLOW_BLUR} result="b" />
            <feMerge>
              <feMergeNode in="b" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        <circle cx={c} cy={c} r={g.segR} fill="none" stroke="var(--muted)" strokeOpacity={0.35} strokeWidth={g.segW} />
        {groups.map(({ group: host, start, end }) => (
          <path
            key={host.id}
            d={arcPath(c, c, g.hostR, start + 0.5, end - 0.5)}
            data-host-arc={host.status}
            fill="none"
            stroke={host.status === "offline" ? "var(--destructive)" : host.color}
            strokeWidth={g.hostW}
            strokeLinecap="round"
            strokeDasharray={host.status === "offline" ? "5 4" : undefined}
            opacity={0.95}
          />
        ))}
        {flat.map(({ item, host, start: s, end: e, index }, i) => {
          const st = item.state;
          const unreachable = st === "unreachable";
          const off = segmentFan(s, e, g.fan);
          const props = popup.segmentProps(index);
          const active = popup.open && popup.active === index;
          const cell = cells[i]!;
          const edge = Math.max(1, 1.2 * (size / 220));
          return (
            <SegmentMenu
              key={item.id}
              unit={item}
              title={unitName(item)}
              onOpenChange={(open) => {
                if (open) popup.setOpen(false);
                else if (!keyboardMenu.current) popup.hide(0);
              }}
              onCloseAutoFocus={(ev) => {
                ev.preventDefault();
                if (keyboardMenu.current) segEls.current[index]?.focus();
                keyboardMenu.current = false;
              }}
            >
              {/* Menu trigger for the hit cell and the drawn segment together. */}
              <g data-segment-root={index} onClick={props.onClick}>
                <path d={sectorPath(c, c, band.r0, band.r1, cell.start, cell.end)} data-hit={index} fill="none" fillRule="evenodd" pointerEvents="all" aria-hidden className="cursor-pointer" onPointerEnter={props.onPointerEnter} />
                <g className="ring-seg-group" data-fanned={(fanned && active) || undefined} style={{ "--fx": `${off.x}px`, "--fy": `${off.y}px` } as React.CSSProperties}>
                  <path
                    ref={(el) => {
                      props.ref(el);
                      segEls.current[index] = el;
                    }}
                    d={arcPath(c, c, g.segR, s, e)}
                    data-runtime={st}
                    data-segment={index}
                    role="button"
                    tabIndex={props.tabIndex}
                    aria-label={t("segment", { name: unitName(item), host: host.name, state: unreachable ? t("unreachable") : ts(st) })}
                    aria-describedby={active ? infoId : undefined}
                    aria-keyshortcuts="Shift+F10"
                    fill="none"
                    stroke={unreachable ? UNREACHABLE_FILL : TONE_VAR[FLEET_TONE[st]]}
                    strokeWidth={g.segW}
                    strokeLinecap="butt"
                    filter={st === "unhealthy" ? `url(#${glowId})` : undefined}
                    opacity={!fan && active ? 0.8 : undefined}
                    className="ring-seg cursor-pointer outline-none"
                    onPointerEnter={props.onPointerEnter}
                    onFocus={props.onFocus}
                    onBlur={props.onBlur}
                  />
                  {unreachable && (
                    <path
                      d={sectorPath(c, c, g.segR - g.segW / 2 + edge / 2, g.segR + g.segW / 2 - edge / 2, s, e)}
                      data-outline="unreachable"
                      fill="none"
                      stroke="var(--destructive)"
                      strokeWidth={edge}
                      strokeDasharray={`${(4 * edge).toFixed(1)} ${(3 * edge).toFixed(1)}`}
                      strokeLinejoin="round"
                      pointerEvents="none"
                      aria-hidden
                    />
                  )}
                </g>
              </g>
            </SegmentMenu>
          );
        })}
        <text x={c} y={c - g.valueSize * 0.07} textAnchor="middle" className="pointer-events-none fill-foreground font-semibold tabular-nums" style={{ fontSize: g.valueSize }}>
          {center.value}
        </text>
        <text x={c} y={c + g.valueSize * 0.67} textAnchor="middle" className="pointer-events-none fill-muted-foreground" style={{ fontSize: g.captionSize }}>
          {center.caption}
        </text>
      </svg>
      {popup.open &&
        popup.current &&
        createPortal(
          <div
            ref={infoRef}
            id={infoId}
            role="tooltip"
            aria-label={t("popupLabel", { name: unitName(popup.current.item) })}
            data-slot="ring-info"
            className="pointer-events-none fixed top-0 left-0 z-50 w-72"
            style={{ visibility: "hidden" }}
          >
            {/* Position lives on the outer element's transform; the fade only touches opacity, so the card never travels. */}
            <div className="animate-fade-in overflow-hidden rounded-2xl border border-border/70 bg-popover text-popover-foreground shadow-2xl">
              <SegmentCard unit={popup.current.item} host={popup.current.host} variant="info" />
            </div>
          </div>,
          document.body,
        )}
      <label className="mt-2 flex items-center justify-center gap-2 text-xs text-muted-foreground">
        <input type="checkbox" checked={showUnraid} onChange={(event) => setPreference("showUnraidRing", event.target.checked)} />
        {t("showUnraid")}
      </label>
    </div>
  );
}
