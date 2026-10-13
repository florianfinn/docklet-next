import { ChevronLeft, ChevronRight, GripVertical, MoveDiagonal2, MoveHorizontal, MoveVertical, Scaling } from "lucide-react";
import { useCallback, useState, type KeyboardEvent } from "react";
import { useTranslations } from "use-intl";

import type { ContainerInfo, Host, Stack } from "@/domain/model";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";

import { clampHostArea, DEFAULT_HOST_AREA, HOST_AREA, setFleetView, type CardsPlacement, type HostAreaSize } from "./fleet-view-store";
import { HostPulse } from "./HostPulse";
import { hostAreaMinHeightRem, useHostAreaResize, type ResizeAxis } from "./use-host-area-resize";
import { insertAt, useSortableGrid } from "./use-sortable-grid";

interface HostCardsProps {
  /** Hosts in the dashboard order. */
  hosts: Host[];
  stacks: Stack[];
  containers: ContainerInfo[];
  placement: CardsPlacement;
  /** Saved size, or the preview while the corner handle is dragged. */
  area: HostAreaSize;
  onAreaPreview: (size: HostAreaSize | null) => void;
  editing: boolean;
  animate: boolean;
  duration: number;
}

const RESIZE_KEYS: Record<string, Partial<HostAreaSize>> = { ArrowLeft: { w: -1 }, ArrowRight: { w: 1 }, ArrowUp: { h: -1 }, ArrowDown: { h: 1 } };
const HANDLE = "absolute z-20 grid touch-none place-items-center rounded-md border bg-card text-muted-foreground shadow-sm outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring";

/**
 * Live host cards of the summary on one grid: columns of at least 17 rem
 * that share the area's width, rows that share its height, so a few hosts
 * fill the area instead of leaving a blank band. In edit mode cards move by
 * drag, arrow buttons or Alt + arrows, and the area resizes by its corner
 * handle, the size popup or Alt + Shift + arrows (width only beside the
 * visual). Order and size are saved for the dashboard only.
 */
export function HostCards({ hosts, stacks, containers, placement, area, onAreaPreview, editing, animate, duration }: HostCardsProps) {
  const t = useTranslations("dashboard.hostCards");
  const tr = useTranslations("dashboard.resize");
  const [announce, setAnnounce] = useState("");
  const ids = hosts.map((h) => h.id);
  const onCommit = useCallback((order: string[]) => setFleetView({ hostOrder: order }), []);
  const grid = useSortableGrid<string>({ items: ids, enabled: editing, animate, duration, onCommit });
  const byId = new Map(hosts.map((h) => [h.id, h]));
  const beside = placement === "right";
  const region = t("area");

  const move = (id: string, delta: number) => {
    const i = ids.indexOf(id);
    const target = ids[i + delta];
    if (!target) return;
    const next = insertAt(ids, id, target, delta > 0);
    setFleetView({ hostOrder: next });
    setAnnounce(t("moved", { name: byId.get(id)?.name ?? id, position: next.indexOf(id) + 1, total: next.length }));
  };
  const commitArea = (size: HostAreaSize) => {
    const next = clampHostArea(beside ? size : { ...size, w: area.w });
    setFleetView({ hostArea: next });
    setAnnounce(t("announce", { w: next.w, h: next.h }));
  };
  const resizeBy = (delta: Partial<HostAreaSize> | "reset") => commitArea(delta === "reset" ? DEFAULT_HOST_AREA : { w: area.w + (delta.w ?? 0), h: area.h + (delta.h ?? 0) });
  const onHandle = useHostAreaResize({ resizableWidth: beside, onPreview: onAreaPreview, onCommit: commitArea });
  const resizeKeys = (e: KeyboardEvent, requireModifiers: boolean, axis: ResizeAxis = "both") => {
    if (!editing || (requireModifiers && !(e.altKey && e.shiftKey))) return false;
    const delta = RESIZE_KEYS[e.key];
    if (!delta || (!beside && delta.w) || (axis === "w" && delta.h) || (axis === "h" && delta.w)) return false;
    e.preventDefault();
    e.stopPropagation();
    resizeBy(delta);
    return true;
  };

  return (
    <div
      role="region"
      aria-label={t("region")}
      data-slot="host-cards"
      data-placement={placement}
      data-area={`${area.w}x${area.h}`}
      className={cn("relative flex min-w-0 flex-col gap-2", editing && "rounded-xl outline-2 outline-offset-4 outline-primary/30 outline-dashed")}
      style={{ minHeight: `${hostAreaMinHeightRem(area.h)}rem` }}
    >
      <p className="sr-only" aria-live="polite">
        {announce}
      </p>
      {editing && (
        <div className="flex flex-wrap items-center gap-1 text-xs text-muted-foreground" data-slot="host-area-tools">
          <span className="font-medium text-foreground">{region}</span>
          <span className="tabular-nums">· {t("cells", { w: area.w, h: area.h })}</span>
          <Popover>
            <PopoverTrigger asChild>
              <Button variant="ghost" size="icon" className="size-7" aria-label={tr("open", { name: region, w: area.w, h: area.h })}>
                <Scaling />
              </Button>
            </PopoverTrigger>
            <PopoverContent align="start" className="w-64 p-3" aria-label={tr("title", { name: region })}>
              <p className="mb-2 text-sm font-medium">{t("cells", { w: area.w, h: area.h })}</p>
              <div className="grid grid-cols-2 gap-1.5">
                <Button size="sm" variant="outline" disabled={!beside || area.w <= HOST_AREA.minW} onClick={() => resizeBy({ w: -1 })}>
                  {tr("narrower")}
                </Button>
                <Button size="sm" variant="outline" disabled={!beside || area.w >= HOST_AREA.maxW} onClick={() => resizeBy({ w: 1 })}>
                  {tr("wider")}
                </Button>
                <Button size="sm" variant="outline" disabled={area.h <= HOST_AREA.minH} onClick={() => resizeBy({ h: -1 })}>
                  {tr("lower")}
                </Button>
                <Button size="sm" variant="outline" disabled={area.h >= HOST_AREA.maxH} onClick={() => resizeBy({ h: 1 })}>
                  {tr("higher")}
                </Button>
                <Button size="sm" variant="ghost" className="col-span-2" onClick={() => resizeBy("reset")}>
                  {tr("reset")}
                </Button>
              </div>
              <p className="mt-2 text-[11px] text-muted-foreground">{beside ? t("areaKeys") : t("areaKeysBelow")}</p>
            </PopoverContent>
          </Popover>
          <Button type="button" variant="link" size="sm" className="ml-auto h-7 text-xs" onClick={() => setFleetView({ hostOrder: [] })}>
            {t("reset")}
          </Button>
        </div>
      )}
      <div className="grid min-h-0 flex-1 auto-rows-[minmax(min-content,1fr)] grid-cols-[repeat(auto-fit,minmax(min(100%,17rem),1fr))] gap-3" data-slot="host-card-grid">
        {grid.order.map((id, i) => {
          const h = byId.get(id);
          if (!h) return null;
          const dragging = grid.dragging === id;
          return (
            <div key={id} ref={grid.slotRef(id)} data-host-card={id} className="relative min-w-0">
              {dragging && <div aria-hidden className="absolute inset-0 rounded-xl border-2 border-dashed border-primary/60 bg-primary/8" />}
              <div
                ref={grid.cardRef(id)}
                data-dragging={dragging || undefined}
                tabIndex={editing ? 0 : undefined}
                aria-roledescription={editing ? t("roleDescription") : undefined}
                aria-label={editing ? h.name : undefined}
                onPointerDown={(e) => grid.onPointerDown(id, e)}
                onKeyDown={(e) => {
                  if (!editing || !e.altKey) return;
                  if (e.shiftKey) {
                    resizeKeys(e, true);
                    return;
                  }
                  const delta = { ArrowLeft: -1, ArrowUp: -1, ArrowRight: 1, ArrowDown: 1 }[e.key];
                  if (!delta) return;
                  e.preventDefault();
                  move(id, delta);
                }}
                className={cn("h-full rounded-xl outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50", editing && "cursor-grab touch-none ring-2 ring-primary/15 select-none", dragging && "relative z-30 cursor-grabbing shadow-2xl")}
              >
                <HostPulse host={h} stacks={stacks.filter((s) => s.hostId === id).length} containers={containers.filter((c) => c.hostId === id).length} className={cn(editing && "border-dashed border-primary/50")} />
                {editing && (
                  <div className="absolute top-2 right-2 flex items-center gap-0.5 rounded-lg border bg-card/90 p-0.5 shadow-sm">
                    <span data-drag-handle className="grid size-6 cursor-grab place-items-center text-muted-foreground" aria-hidden>
                      <GripVertical className="size-3.5" />
                    </span>
                    <Button variant="ghost" size="icon" className="size-6" disabled={i === 0} onClick={() => move(id, -1)} aria-label={t("earlier", { name: h.name })}>
                      <ChevronLeft />
                    </Button>
                    <Button variant="ghost" size="icon" className="size-6" disabled={i === grid.order.length - 1} onClick={() => move(id, 1)} aria-label={t("later", { name: h.name })}>
                      <ChevronRight />
                    </Button>
                  </div>
                )}
              </div>
            </div>
          );
        })}
      </div>
      {editing && (
        <>
          <button
            type="button"
            data-host-area-handle
            aria-label={tr("handle", { name: region })}
            onPointerDown={(e) => onHandle(area, e)}
            onKeyDown={(e) => void resizeKeys(e, false)}
            className={cn(HANDLE, "-bottom-3 size-7", beside ? "-left-3 cursor-nesw-resize" : "-right-3 cursor-ns-resize")}
          >
            <MoveDiagonal2 className={cn("size-4", beside && "-scale-x-100")} aria-hidden />
          </button>
          {beside && (
            <button
              type="button"
              data-host-area-handle="w"
              aria-label={tr("handleWidth", { name: region })}
              onPointerDown={(e) => onHandle(area, e, "w")}
              onKeyDown={(e) => void resizeKeys(e, false, "w")}
              className={cn(HANDLE, "top-1/2 -left-3 h-9 w-6 -translate-y-1/2 cursor-ew-resize")}
            >
              <MoveHorizontal className="size-3.5" aria-hidden />
            </button>
          )}
          <button
            type="button"
            data-host-area-handle="h"
            aria-label={tr("handleHeight", { name: region })}
            onPointerDown={(e) => onHandle(area, e, "h")}
            onKeyDown={(e) => void resizeKeys(e, false, "h")}
            className={cn(HANDLE, "-bottom-3 left-1/2 h-6 w-9 -translate-x-1/2 cursor-ns-resize")}
          >
            <MoveVertical className="size-3.5" aria-hidden />
          </button>
        </>
      )}
    </div>
  );
}
