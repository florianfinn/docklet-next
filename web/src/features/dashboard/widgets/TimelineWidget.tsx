import { memo } from "react";
import { useLocale, useTranslations } from "use-intl";

import { formatTime } from "@/domain/format";
import type { EventKind, Host, OpsEvent, Tone } from "@/domain/model";
import { RECENT_EVENT_LIMIT } from "@/data/adapter";
import { openActivity } from "@/features/activity/activity-store";
import { ToneDot } from "@/shared/components/tone";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";
import { timelineBuckets, type TimelineBucket } from "./timeline-buckets";

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

const TICKS = [24, 18, 12, 6, 0];
const SHOWN = 5;

type Props = { events: OpsEvent[]; hosts: Host[]; now: number; total?: number };
const lanes = (hosts: Host[]) => JSON.stringify(hosts.map((h) => [h.id, h.name, h.color]));

/**
 * 24-hour lanes per host with one marker per non-empty 15-minute slot (at most 96 per lane), a count badge and the
 * slot's newest entries in its tooltip. The widget renders only when its events, lanes or time change, not on every
 * metrics or host-observation update of the Dashboard.
 */
export const TimelineWidget = memo(function TimelineWidget({ events, hosts, now, total }: Props) {
  const t = useTranslations("dashboard.timeline");
  const buckets = timelineBuckets(events, hosts.map((h) => h.id), now);
  const shown = [...buckets.values()].flat();
  // The bounded window may hold no event of the shown hosts while it was truncated: keep the full-history link.
  if (!shown.length) return <div className="flex flex-col gap-1 text-sm text-muted-foreground"><p>{t("empty")}</p><p className="flex text-xs"><RecentEventsLimit total={total} /></p></div>;
  const kinds = [...new Set(shown.flatMap((b) => b.events.map((e) => e.kind)))];

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
              {(buckets.get(h.id) ?? []).map((b) => <Marker key={b.key} bucket={b} />)}
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
      <div className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {kinds.map((k) => (
          <span key={k} className="inline-flex items-center gap-1.5">
            <ToneDot tone={KIND_TONE[k]} />
            {t(`legend.${k}`)}
          </span>
        ))}
        <RecentEventsLimit total={total} />
      </div>
    </div>
  );
}, (a, b) => a.events === b.events && a.now === b.now && a.total === b.total && lanes(a.hosts) === lanes(b.hosts));

/** The Dashboard reads at most 500 events of the last 24 hours; more are reachable in the Activity Center. */
export function RecentEventsLimit({ total }: { total?: number }) {
  const t = useTranslations("dashboard.recent");
  if (total === undefined || total <= RECENT_EVENT_LIMIT) return null;
  return (
    <span className="ms-auto inline-flex items-center gap-1.5">
      {t("truncated", { limit: RECENT_EVENT_LIMIT })}
      <button type="button" className="text-primary-text hover:underline" onClick={() => openActivity(undefined, "events")}>
        {t("showAll")}
      </button>
    </span>
  );
}

/** A slot's marker; the locale subscription re-renders it because formatTime reads the active locale. */
const Marker = memo(function Marker({ bucket }: { bucket: TimelineBucket }) {
  const t = useTranslations("dashboard.timeline");
  useLocale();
  const [newest] = bucket.events;
  const count = bucket.events.length;
  const time = formatTime(bucket.at);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          data-tone={KIND_TONE[newest!.kind]}
          aria-label={count > 1 ? `${time} · ${t("count", { count })} · ${newest!.title}` : `${time} · ${newest!.title}`}
          className="absolute top-1/2 size-3.5 -translate-x-1/2 -translate-y-1/2 rounded-full border-2 border-card bg-(--tone) shadow-[0_0_calc(10px*var(--glow))_var(--tone)] outline-none transition-transform duration-(--dur-fast) hover:scale-125 focus-visible:scale-125 focus-visible:ring-2 focus-visible:ring-ring"
          style={{ left: `${bucket.left}%` }}
        >
          {count > 1 && <span aria-hidden className="absolute -top-2.5 left-2 rounded-full bg-card px-1 text-[9.5px] leading-tight tabular">{count}</span>}
        </button>
      </TooltipTrigger>
      <TooltipContent className="max-w-64">
        <SlotEntries events={bucket.events} />
        {count > SHOWN && <div className="mt-1 text-muted-foreground">{t("more", { count: count - SHOWN })}</div>}
      </TooltipContent>
    </Tooltip>
  );
}, (a, b) => a.bucket.key === b.bucket.key && a.bucket.left === b.bucket.left && entries(a.bucket) === entries(b.bucket));
// A refetch builds new slot objects; only a slot whose entries changed renders again.
const entries = (bucket: TimelineBucket) => bucket.events.map((e) => `${e.id}\u0000${e.title}\u0000${e.at}\u0000${e.kind}`).join("\u0001");

/** Formats the slot's entries only while its tooltip is open. */
function SlotEntries({ events }: { events: OpsEvent[] }) {
  return (
    <ul className="flex flex-col gap-1">
      {events.slice(0, SHOWN).map((e) => (
        <li key={e.id}><span className="font-medium">{formatTime(e.at)}</span> {e.title}</li>
      ))}
    </ul>
  );
}
