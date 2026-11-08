import { memo } from "react";
import { useTranslations } from "use-intl";

import { formatTime } from "@/domain/format";
import type { EventKind, Host, OpsEvent, Tone } from "@/domain/model";
import { ToneDot } from "@/shared/components/tone";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

const KIND_TONE: Record<EventKind, Tone> = {
  activity: "gray",
  deploy: "accent",
  update: "warn",
  restart: "info",
  health: "err",
  host: "orange",
  backup: "ok",
  scan: "violet",
  config: "gray",
};

const SPAN = 24 * 3_600_000;
const TICKS = [24, 18, 12, 6, 0];

type Props = { events: OpsEvent[]; hosts: Host[]; now: number };
const lanes = (hosts: Host[]) => JSON.stringify(hosts.map((h) => [h.id, h.name, h.color]));

/**
 * 24-hour lanes per host; markers are focusable buttons with details. A busy fleet holds
 * thousands of markers, so the widget renders only when its events, lanes or time change,
 * not on every metrics or host-observation update of the Dashboard.
 */
export const TimelineWidget = memo(function TimelineWidget({ events, hosts, now }: Props) {
  const t = useTranslations("dashboard.timeline");
  const recent = events.filter((e) => now - new Date(e.at).getTime() <= SPAN);
  if (!recent.length) return <p className="text-sm text-muted-foreground">{t("empty")}</p>;
  const pos = (iso: string) => `${Math.max(1, Math.min(99, 100 - ((now - new Date(iso).getTime()) / SPAN) * 100))}%`;
  const kinds = [...new Set(recent.map((e) => e.kind))];

  return (
    <div className="flex h-full flex-col gap-3">
      <div className="grid flex-1 grid-cols-[64px_minmax(0,1fr)] gap-x-2 rounded-xl border bg-background/40 p-3">
        <div className="flex flex-col justify-around">
          {hosts.map((h) => (
            <span key={h.id} className="flex items-center gap-1.5 truncate text-xs text-muted-foreground">
              <i className="size-2 shrink-0 rounded-[2px]" style={{ background: h.color }} aria-hidden />
              {h.name}
            </span>
          ))}
        </div>
        <div className="relative flex flex-col justify-around">
          {[25, 50, 75].map((p) => (
            <span key={p} aria-hidden className="absolute inset-y-0 w-px bg-border/50" style={{ left: `${p}%` }} />
          ))}
          <span aria-hidden className="absolute inset-y-0 right-0 w-px bg-primary/60" />
          {hosts.map((h) => (
            <div key={h.id} className="relative h-7">
              <span aria-hidden className="absolute inset-x-0 top-1/2 h-px bg-border" />
              {recent
                .filter((e) => e.hostId === h.id)
                .map((e) => <Marker key={e.id} event={e} left={pos(e.at)} />)}
            </div>
          ))}
        </div>
        <span />
        <div className="mt-1 flex justify-between text-[10.5px] text-muted-foreground tabular">
          {TICKS.map((h) => (
            <span key={h}>{h === 0 ? t("now") : t("hoursAgo", { h })}</span>
          ))}
        </div>
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {kinds.map((k) => (
          <span key={k} className="inline-flex items-center gap-1.5">
            <ToneDot tone={KIND_TONE[k]} />
            {t(`legend.${k}`)}
          </span>
        ))}
      </div>
    </div>
  );
}, (a, b) => a.events === b.events && a.now === b.now && lanes(a.hosts) === lanes(b.hosts));

/** Structural sharing keeps unchanged events identical, so an event refetch rebuilds only new markers. */
const Marker = memo(function Marker({ event, left }: { event: OpsEvent; left: string }) {
  const time = formatTime(event.at);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-tone={KIND_TONE[event.kind]}
          aria-label={`${time} · ${event.title}`}
          className="absolute top-1/2 size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-card bg-(--tone) shadow-[0_0_calc(10px*var(--glow))_var(--tone)] outline-none transition-transform duration-(--dur-fast) hover:scale-125 focus-visible:scale-125 focus-visible:ring-2 focus-visible:ring-ring"
          style={{ left }}
        />
      </TooltipTrigger>
      <TooltipContent className="max-w-64">
        <div className="font-medium">{time}</div>
        {event.title}
      </TooltipContent>
    </Tooltip>
  );
});
