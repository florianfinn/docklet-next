import { useRef } from "react";
import { useTranslations } from "use-intl";

import type { Host } from "@/domain/model";
import { ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Popover, PopoverAnchor, PopoverContent } from "@/shared/ui/popover";

import { TONE_VAR, useSegmentPopup, type FleetSegment } from "./fleet-segments";
import { SegmentCard } from "./SegmentCard";
import { useUnitName } from "./unit-name";
import { FLEET_TONE, type FleetUnit } from "./fleet-status";

interface FleetLineProps {
  hosts: Host[];
  units: FleetUnit[];
  label: string;
}

/**
 * The ring unrolled into one status line per host: one segment per stack or
 * origin group in the same fleet states and colours (red dashed for a
 * disconnected host), with running/total stacks at the end. It shows current states only, no history.
 * Segments share the ring's keyboard model and popup.
 */
export function FleetLine({ hosts, units, label }: FleetLineProps) {
  const t = useTranslations("dashboard.ring");
  const ts = useTranslations("common.status");
  const tc = useTranslations("common.host");
  const unitName = useUnitName();
  const rows = hosts.map((h) => ({ host: h, items: units.filter((u) => u.hostId === h.id) })).filter((r) => r.items.length);
  const segments: FleetSegment[] = [];
  for (const r of rows) for (const item of r.items) segments.push({ item, host: r.host, state: item.state, index: segments.length });
  const groupRef = useRef<HTMLDivElement>(null);
  const contentRef = useRef<HTMLDivElement>(null);
  const popup = useSegmentPopup(segments, { group: groupRef, content: contentRef });
  const els = useRef<(HTMLElement | null)[]>([]);
  const anchor = useRef({ getBoundingClientRect: () => els.current[popup.active]?.getBoundingClientRect() ?? new DOMRect() });

  return (
    <Popover open={popup.open} onOpenChange={(o) => !o && popup.setOpen(false)}>
      <PopoverAnchor virtualRef={anchor} />
      <div
        ref={groupRef}
        role="group"
        aria-label={label}
        aria-roledescription={t("lineRoleDescription")}
        data-slot="fleet-line"
        onKeyDown={popup.onKeyDown}
        onPointerLeave={(e) => e.pointerType === "mouse" && popup.hide()}
        className="flex w-full min-w-0 flex-col gap-3"
      >
        {rows.map(({ host, items }) => {
          const running = items.filter((u) => u.state === "running").length;
          return (
            <div key={host.id} className="grid grid-cols-[minmax(5rem,8rem)_minmax(0,1fr)_auto] items-center gap-3" data-line-host={host.id}>
              <span className="flex min-w-0 items-center gap-1.5 text-xs font-medium">
                <i className="size-2.5 shrink-0 rounded-[3px]" style={{ background: host.color }} aria-hidden />
                <span className="truncate">{host.name}</span>
                {host.status === "offline" && (
                  <ToneBadge tone="err" className="h-4.5 px-1 text-[10px]">
                    {tc("offline")}
                  </ToneBadge>
                )}
              </span>
              <div className="flex h-6 items-center gap-[3px]">
                {items.map((item) => {
                  const seg = segments.find((s) => s.item.id === item.id)!;
                  const props = popup.segmentProps(seg.index);
                  const unreachable = seg.state === "unreachable";
                  const active = popup.open && popup.active === seg.index;
                  return (
                    <button
                      key={item.id}
                      type="button"
                      ref={(el) => {
                        props.ref(el);
                        els.current[seg.index] = el;
                      }}
                      data-runtime={seg.state}
                      data-segment={seg.index}
                      tabIndex={props.tabIndex}
                      aria-label={t("segment", { name: unitName(item), host: host.name, state: unreachable ? t("unreachable") : ts(seg.state) })}
                      onClick={props.onClick}
                      onPointerEnter={props.onPointerEnter}
                      onFocus={props.onFocus}
                      onBlur={props.onBlur}
                      style={{ "--seg": TONE_VAR[FLEET_TONE[seg.state]] } as React.CSSProperties}
                      className={cn(
                        "line-seg h-4 min-w-3 flex-1 rounded-[3px] outline-none focus-visible:ring-2 focus-visible:ring-ring",
                        unreachable ? "border-2 border-dashed border-(--seg) bg-transparent" : "bg-(--seg)",
                        seg.state === "unhealthy" && "shadow-[0_0_8px_var(--seg)]",
                      )}
                      data-active={active || undefined}
                    />
                  );
                })}
              </div>
              <span className="text-xs text-muted-foreground tabular-nums">{t("lineCount", { running, total: items.length })}</span>
            </div>
          );
        })}
      </div>
      {popup.current && (
        <PopoverContent
          ref={contentRef}
          {...popup.contentProps}
          side="bottom"
          align="center"
          sideOffset={8}
          updatePositionStrategy="always"
          aria-label={t("popupLabel", { name: unitName(popup.current.item) })}
          className="w-72 overflow-hidden rounded-2xl border-border/70 p-0 shadow-2xl"
        >
          <SegmentCard unit={popup.current.item} host={popup.current.host} />
        </PopoverContent>
      )}
    </Popover>
  );
}
