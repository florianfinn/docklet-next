import { ChevronLeft, ChevronRight, EyeOff, GripVertical, MoveDiagonal2, Scaling, Settings2 } from "lucide-react";
import type { PointerEvent, ReactNode, Ref } from "react";
import { useTranslations } from "use-intl";

import type { Tone } from "@/domain/model";
import { IconTile } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Card, CardAction, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Popover, PopoverContent, PopoverTrigger } from "@/shared/ui/popover";

import type { WidgetSize } from "../dashboard-layout";
import { spanStyle } from "../use-widget-grid";

export interface WidgetChrome {
  editing: boolean;
  onMove: (delta: number) => void;
  onHide: () => void;
  first: boolean;
  last: boolean;
  dragging: boolean;
  /** Another card is being dragged; this one is a possible drop target. */
  dropTarget: boolean;
  onPointerDown: (e: PointerEvent) => void;
  /** Service widgets open their connection settings. */
  onConfigure?: () => void;
  /** Current size (preview while resizing), the grid's columns and the size limits for this widget. */
  size: WidgetSize;
  columns: number;
  limits: { minW: number; maxW: number; minH: number; maxH: number };
  resizing: boolean;
  onResize: (delta: { w?: number; h?: number } | "reset") => void;
  onResizeHandle: (e: PointerEvent<HTMLElement>) => void;
}

interface WidgetFrameProps {
  id: string;
  title: string;
  description: string;
  icon: ReactNode;
  /** Plain icons get a tone tile; app marks bring their own tile. */
  tone?: Tone;
  /** Accent colour of service widgets. */
  accent?: string;
  action?: ReactNode;
  chrome: WidgetChrome;
  /** Grid cell (measured for drop targets) and the card that moves while dragging. */
  slotRef: Ref<HTMLDivElement>;
  cardRef: Ref<HTMLDivElement>;
  children: ReactNode;
}

/**
 * Widget card on the dashboard grid with in-place edit controls. In edit mode
 * the whole card drags (grip, mouse, pen or touch); arrow buttons and Alt +
 * arrow keys move it. Width and height snap to grid cells: the corner handle,
 * the size popup or Alt + Shift + arrow keys resize it.
 */
export function WidgetFrame({ id, title, description, icon, tone, accent, action, chrome, slotRef, cardRef, children }: WidgetFrameProps) {
  const t = useTranslations("dashboard");
  const { size, limits } = chrome;
  return (
    <div ref={slotRef} data-widget-slot={id} data-size={`${size.w}x${size.h}`} className="relative min-w-0" style={spanStyle(size, chrome.columns)}>
      {chrome.dragging && <div aria-hidden className="absolute inset-0 rounded-xl border-2 border-dashed border-primary/60 bg-primary/8" data-slot="drop-placeholder" />}
      {chrome.resizing && (
        <div aria-hidden data-slot="resize-preview" className="pointer-events-none absolute inset-0 z-40 grid place-items-center rounded-xl border-2 border-dashed border-primary bg-primary/8 text-sm font-semibold text-primary-text">
          {t("resize.cells", { w: size.w, h: size.h })}
        </div>
      )}
      <Card
        ref={cardRef}
        data-widget={id}
        data-dragging={chrome.dragging || undefined}
        aria-labelledby={`w-${id}`}
        aria-roledescription={chrome.editing ? t("customizeSheet.roleDescription") : undefined}
        tabIndex={chrome.editing ? 0 : undefined}
        style={{ viewTransitionName: chrome.dragging ? undefined : `widget-${id}`, ...(accent ? { "--widget-accent": accent } : {}) } as React.CSSProperties}
        onPointerDown={chrome.onPointerDown}
        onKeyDown={(e) => {
          if (!chrome.editing || !e.altKey) return;
          if (e.shiftKey) {
            const delta = { ArrowLeft: { w: -1 }, ArrowRight: { w: 1 }, ArrowUp: { h: -1 }, ArrowDown: { h: 1 } }[e.key];
            if (delta) {
              e.preventDefault();
              chrome.onResize(delta);
            }
            return;
          }
          if (e.key === "ArrowLeft" || e.key === "ArrowUp") {
            e.preventDefault();
            chrome.onMove(-1);
          } else if (e.key === "ArrowRight" || e.key === "ArrowDown") {
            e.preventDefault();
            chrome.onMove(1);
          }
        }}
        className={cn(
          "lift relative h-full gap-3 overflow-hidden py-4 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
          accent && "before:absolute before:inset-x-0 before:top-0 before:h-0.5 before:bg-(--widget-accent)",
          chrome.editing && "cursor-grab touch-none border-dashed border-primary/50 ring-2 ring-primary/15 select-none",
          chrome.dropTarget && "transition-[box-shadow,border-color] duration-(--dur-fast) hover:border-primary hover:ring-primary/35",
          chrome.dragging && "z-30 cursor-grabbing border-solid border-primary shadow-2xl ring-primary/40 will-change-transform",
        )}
      >
        <CardHeader className="px-4">
          <CardTitle id={`w-${id}`} className="flex items-center gap-2.5">
            {tone ? (
              <IconTile tone={tone} size="sm" className="tilt">
                {icon}
              </IconTile>
            ) : (
              icon
            )}
            <span className="truncate">{title}</span>
          </CardTitle>
          <CardDescription className="sr-only">{description}</CardDescription>
          <CardAction className="flex items-center gap-1">
            {chrome.editing ? (
              <>
                <span data-drag-handle className="grid size-7 cursor-grab touch-none place-items-center rounded-md text-muted-foreground hover:bg-muted" aria-hidden>
                  <GripVertical className="size-4" />
                </span>
                <Popover>
                  <PopoverTrigger asChild>
                    <Button variant="ghost" size="icon" className="size-7" aria-label={t("resize.open", { name: title, w: size.w, h: size.h })}>
                      <Scaling />
                    </Button>
                  </PopoverTrigger>
                  <PopoverContent align="end" className="w-60 p-3" aria-label={t("resize.title", { name: title })}>
                    <p className="mb-2 text-sm font-medium">{t("resize.cells", { w: size.w, h: size.h })}</p>
                    <div className="grid grid-cols-2 gap-1.5">
                      <Button size="sm" variant="outline" disabled={size.w <= limits.minW} onClick={() => chrome.onResize({ w: -1 })}>
                        {t("resize.narrower")}
                      </Button>
                      <Button size="sm" variant="outline" disabled={size.w >= limits.maxW} onClick={() => chrome.onResize({ w: 1 })}>
                        {t("resize.wider")}
                      </Button>
                      <Button size="sm" variant="outline" disabled={size.h <= limits.minH} onClick={() => chrome.onResize({ h: -1 })}>
                        {t("resize.lower")}
                      </Button>
                      <Button size="sm" variant="outline" disabled={size.h >= limits.maxH} onClick={() => chrome.onResize({ h: 1 })}>
                        {t("resize.higher")}
                      </Button>
                      <Button size="sm" variant="ghost" className="col-span-2" onClick={() => chrome.onResize("reset")}>
                        {t("resize.reset")}
                      </Button>
                    </div>
                    <p className="mt-2 text-[11px] text-muted-foreground">{t("resize.keys")}</p>
                  </PopoverContent>
                </Popover>
                {chrome.onConfigure && (
                  <Button variant="ghost" size="icon" className="size-7" onClick={chrome.onConfigure} aria-label={t("customizeSheet.configure", { name: title })}>
                    <Settings2 />
                  </Button>
                )}
                <Button variant="ghost" size="icon" className="size-7" disabled={chrome.first} onClick={() => chrome.onMove(-1)} aria-label={t("customizeSheet.up", { name: title })}>
                  <ChevronLeft />
                </Button>
                <Button variant="ghost" size="icon" className="size-7" disabled={chrome.last} onClick={() => chrome.onMove(1)} aria-label={t("customizeSheet.down", { name: title })}>
                  <ChevronRight />
                </Button>
                <Button variant="ghost" size="icon" className="size-7" onClick={chrome.onHide} aria-label={t("customizeSheet.hide", { name: title })}>
                  <EyeOff />
                </Button>
              </>
            ) : (
              action
            )}
          </CardAction>
        </CardHeader>
        <CardContent className={cn("min-h-0 flex-1 overflow-y-auto px-4", chrome.editing && "pointer-events-none opacity-60 select-none")}>{children}</CardContent>
        {chrome.editing && (
          <button
            type="button"
            data-resize-handle
            aria-label={t("resize.handle", { name: title })}
            onPointerDown={chrome.onResizeHandle}
            onKeyDown={(e) => {
              const delta = { ArrowLeft: { w: -1 }, ArrowRight: { w: 1 }, ArrowUp: { h: -1 }, ArrowDown: { h: 1 } }[e.key];
              if (!delta) return;
              e.preventDefault();
              e.stopPropagation();
              chrome.onResize(delta);
            }}
            className="absolute right-1 bottom-1 z-20 grid size-7 cursor-nwse-resize touch-none place-items-center rounded-md text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
          >
            <MoveDiagonal2 className="size-4" aria-hidden />
          </button>
        )}
      </Card>
    </div>
  );
}
