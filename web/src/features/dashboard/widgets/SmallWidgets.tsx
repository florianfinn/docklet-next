import { Archive, CalendarClock, ExternalLink, RefreshCw, ScanSearch, ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { useScanAction } from "@/features/security/use-scan-action";
import { RecentEventsLimit } from "./TimelineWidget";
import { useCapability } from "@/data/use-capability";
import { useMetrics } from "@/data/live";
import { useCapacity } from "@/data/queries";
import { formatCountdown, formatGb, formatRelative } from "@/domain/format";
import type { HostCapacity } from "@/domain/history";
import { capacitySegments } from "../capacity-line";
import { jobHref } from "@/domain/job-links";
import { formatNumber } from "@/domain/locale";
import type { EventLevel, Host, JobKind, OpsEvent, ScheduledJob, SecurityOverview, Severity, Stack } from "@/domain/model";
import { STACK_STATUS_TONE, stackStatus } from "@/domain/status";
import { HostChip } from "@/shared/components/host-chip";
import { Meter } from "@/shared/components/meter";
import { IconTile, ToneDot } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { ScrollArea } from "@/shared/ui/scroll-area";

export function CapacityWidget({ hosts }: { hosts: Host[] }) {
  const status = useTranslations("common.host");
  const t = useTranslations("dashboard.capacity");
  const td = useTranslations("dashboard");
  const metrics = useMetrics((s) => s?.hosts);
  const capacity = useCapacity();
  return (
    <ScrollArea className="h-full">
    <ul className="flex flex-col gap-3.5 pr-2">
      {hosts.map((h) => {
        const m = metrics?.[h.id];
        const total = m?.diskTotalGb ?? h.diskTotalGb;
        if (!m || m.diskUnavailable) return <li key={h.id}>{h.name}: {td("metricsUnavailable")}</li>;
        const used = m.disk / 100 * total;
        const trend = capacity.data?.find((c) => c.hostId === h.id);
        const growth = trend?.growthPerDay;
        const days = trend?.daysUntilFull === undefined ? Infinity : Math.round(trend.daysUntilFull);
        const tone = days < 30 ? "err" : days < 90 ? "warn" : "ok";
        return (
          <li key={h.id} className="flex flex-col gap-1.5" data-capacity-host={h.id}>
            <Meter label={h.name} value={m?.disk ?? 0} color={h.color} sub={t("used", { used: formatGb(used), total: formatGb(total) })} />
            {trend && <CapacityLine trend={trend} color={h.color} label={t("line")} />}
            {trend?.diskSource && <span className="text-[11px] text-muted-foreground" data-capacity-source={trend.diskSource}>{t("measuredAt", { path: trend.diskSource })}</span>}
            <div className="flex items-center justify-between text-[11px]">
              <span data-tone={h.status !== "online" ? "gray" : tone} className="font-medium text-(--tone)">
                {h.status === "standby" ? status("standby") : m.inventory?.stale || h.status !== "online" ? t("offline") : growth === undefined ? "" : Number.isFinite(days) && days < 365 ? t("days", { days }) : t("plenty")}
              </span>
              <span className="text-muted-foreground tabular" data-growth={growth === undefined ? "unavailable" : "known"}>{growth === undefined ? td("growthUnavailable") : t("growth", { gb: formatNumber(growth / GIB, { maximumFractionDigits: 1 }) })}</span>
            </div>
          </li>
        );
      })}
    </ul>
    </ScrollArea>
  );
}

const GIB = 1024 ** 3;

/** Disk usage of the last 30 days as share of the current disk size. */
function CapacityLine({ trend, color, label }: { trend: HostCapacity; color: string; label: string }) {
  const segments = capacitySegments(trend);
  if (trend.series.length < 2 || !segments.length) return null;
  const coordinates = (points: Array<[number, number]>) => points.map(([x, y]) => `${Math.round(x * 100) / 100},${Math.round(y * 100) / 100}`).join(" ");
  return (
    <svg viewBox="0 0 100 24" preserveAspectRatio="none" className="h-6 w-full overflow-visible" role="img" aria-label={label} data-capacity-line={trend.series.length} data-capacity-segments={segments.length}>
      {segments.map((points) => points.length > 1
        ? <polyline key={points[0]![0]} data-points={coordinates(points)} points={coordinates(points)} fill="none" stroke={color} strokeWidth={1.5} vectorEffect="non-scaling-stroke" strokeLinejoin="round" />
        : <circle key={points[0]![0]} data-points={coordinates(points)} cx={points[0]![0]} cy={points[0]![1]} r={0.8} fill={color} />)}
    </svg>
  );
}

const JOB_ICON: Record<JobKind, ReactNode> = {
  backup: <Archive />,
  scan: <ScanSearch />,
  "update-check": <RefreshCw />,
  "auto-update": <CalendarClock />,
};
const JOB_TONE = { backup: "ok", scan: "violet", "update-check": "info", "auto-update": "warn" } as const;

export function ScheduleWidget({ jobs, hosts, now }: { jobs: ScheduledJob[]; hosts: Map<string, Host>; now: number }) {
  const t = useTranslations("dashboard.schedule");
  const tu = useTranslations("common.stackUpdates");
  const sorted = [...jobs].sort((a, b) => (a.nextRunAt ?? "").localeCompare(b.nextRunAt ?? ""));
  if (!sorted.length) return <p className="text-sm text-muted-foreground">{t("empty")}</p>;
  return (
    <ScrollArea className="h-full">
      <ol className="relative flex flex-col gap-1 pr-2 before:absolute before:top-3 before:bottom-3 before:left-[13px] before:w-px before:bg-border">
        {sorted.map((j) => {
          const when = j.unavailable ? tu("unavailable") : j.enabled === false ? tu("disabled") : j.nextRunAt ? formatCountdown(j.nextRunAt, now) : tu("unknown");
          const title = j.title || t(`kinds.${j.kind}`);
          const target = j.kind === "backup" && !j.volumeId ? "backupList" : j.kind === "auto-update" && !j.stackId ? "autoPolicies" : j.kind;
          return (
            <li key={j.id} className="relative">
              {/* A real link: it opens the page that configures this job, also in a new tab. */}
              <Link
                to={jobHref(j)}
                data-glow="row"
                data-job={j.kind}
                aria-label={t("open", { title, when, target: t(`targets.${target}`) })}
                className="group/job flex items-center gap-3 rounded-lg py-1.5 pr-1 outline-none hover:bg-muted/40 focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                <IconTile tone={JOB_TONE[j.kind]} size="sm" className="z-10 ring-4 ring-card">
                  {JOB_ICON[j.kind]}
                </IconTile>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm group-hover/job:underline group-hover/job:underline-offset-4">{title}</div>
                  <div className="flex items-center gap-2 text-xs text-muted-foreground">
                    {t(`kinds.${j.kind}`)} · {j.cadence}
                    <HostChip host={hosts.get(j.hostId ?? "")} />
                  </div>
                </div>
                <span className="shrink-0 text-xs font-medium text-primary-text tabular">{when}</span>
              </Link>
            </li>
          );
        })}
      </ol>
    </ScrollArea>
  );
}

const SEV: { key: Severity; tone: "err" | "orange" | "warn" | "lime" | "gray" }[] = [
  { key: "critical", tone: "err" },
  { key: "high", tone: "orange" },
  { key: "medium", tone: "warn" },
  { key: "low", tone: "lime" },
  { key: "unknown", tone: "gray" },
];

export function SecurityWidget({ security, visibleHostIds }: { security: SecurityOverview; visibleHostIds: Set<string> }) {
  const t = useTranslations("dashboard.security");
  const navigate = useNavigate();
  const scan = useScanAction();
  const scanCapability = useCapability("setScanEnabled");
  if (!security.enabled) {
    return (
      <div className="flex h-full flex-col items-start justify-center gap-3">
        <p className="text-sm text-muted-foreground">{t("disabled")}</p>
        <Button size="sm" variant="outline" disabled={!scanCapability.available || scan.busy} title={scanCapability.reason} onClick={() => void scan.enable()}>
          <ShieldCheck data-icon="inline-start" /> {t("enable")}
        </Button>
        {scan.note && <p role="status">{scan.note}</p>}
        {scan.error && <p role="alert">{scan.error}</p>}
      </div>
    );
  }
  const unavailable = security.unavailable?.some((h) => visibleHostIds.has(h.hostId)) || security.images.some((i) => visibleHostIds.has(i.hostId) && i.unavailable);
  const images = security.images.filter((i) => visibleHostIds.has(i.hostId) && !i.unavailable && i.scannedAt);
  const totals = SEV.map((s) => ({ ...s, n: images.reduce((n, i) => n + (i.counts[s.key] ?? 0), 0) }));
  const actionable = images.reduce((n, i) => n + i.counts.critical + i.counts.high, 0);
  const all = totals.reduce((n, s) => n + s.n, 0);
  return (
    <div className="flex h-full flex-col gap-4">
      {unavailable && <p role="status" className="text-sm text-muted-foreground">{t("unavailable")}</p>}
      <div className="grid grid-cols-[repeat(auto-fit,minmax(4rem,1fr))] gap-2">
        {totals.map((s) => (
          <div key={s.key} data-tone={s.tone} className="rounded-xl border border-(--tone)/25 bg-(--tone)/8 px-2 py-3 text-center">
            <div className="text-2xl font-semibold text-(--tone) tabular">{!images.length ? "–" : s.n}</div>
            <div className="text-[11px] text-muted-foreground">{t(`severities.${s.key}`)}</div>
          </div>
        ))}
      </div>
      <div className="flex h-2.5 gap-0.5 overflow-hidden rounded-full" aria-hidden>
        {totals.map((s) => (s.n ? <span key={s.key} data-tone={s.tone} className="rounded-full bg-(--tone)" style={{ flexGrow: s.n / Math.max(1, all) }} /> : null))}
      </div>
      <p data-tone={!images.length ? "gray" : actionable ? "err" : "ok"} className="text-sm text-(--tone)">
        {!images.length ? t(unavailable ? "unavailable" : "notScanned") : t("running", { count: actionable })}
      </p>
      <Button size="sm" variant="outline" className="mt-auto self-start" onClick={() => navigate("/security")}>
        {t("open")}
      </Button>
    </div>
  );
}

export function AppsWidget({ stacks, hosts }: { stacks: Stack[]; hosts: Map<string, Host> }) {
  const t = useTranslations("dashboard.apps");
  // Most stacks have no address; parsing only present ones keeps Dashboard renders free of thrown errors.
  const withUrl = stacks.filter((s) => {
    try { return !!s.url && ["https:", "http:"].includes(new URL(s.url).protocol); } catch { return false; }
  });
  if (!withUrl.length) return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  return (
    <div className="grid grid-cols-2 gap-2">
      {withUrl.map((s) => {
        const host = hosts.get(s.hostId);
        return (
          <a
            key={s.id}
            href={s.url}
            target="_blank"
            rel="noopener noreferrer"
            className="lift group flex min-w-0 items-center gap-2.5 rounded-xl border bg-background/40 p-2.5 text-left"
          >
            <IconTile color={host?.color} size="sm" className="tilt">
              <ExternalLink />
            </IconTile>
            <span className="min-w-0">
              <span className="flex items-center gap-1.5 text-sm font-medium">
                {s.name}
                <ToneDot tone={STACK_STATUS_TONE[stackStatus(s, host)]} />
              </span>
              <span className="block truncate text-[11px] text-muted-foreground">{s.url?.replace(/^https?:\/\//, "")}</span>
            </span>
          </a>
        );
      })}
    </div>
  );
}

const LEVEL_TONE: Record<EventLevel, "err" | "warn" | "ok" | "info"> = { error: "err", warn: "warn", ok: "ok", info: "info" };

export function EventsWidget({ events, hosts, total }: { events: OpsEvent[]; hosts: Map<string, Host>; total?: number }) {
  const t = useTranslations("dashboard.events");
  if (!events.length) return <div className="flex flex-col gap-1 text-sm text-muted-foreground"><p>{t("empty")}</p><p className="flex text-xs"><RecentEventsLimit total={total} /></p></div>;
  return (
    <ScrollArea className="h-full">
      <p className="flex pr-2 text-xs text-muted-foreground"><RecentEventsLimit total={total} /></p>
      <ol className="flex flex-col pr-2">
        {events.slice(0, 12).map((e) => (
          <li key={e.id} className="flex gap-2.5 border-b border-border/50 py-2 last:border-0">
            <ToneDot tone={LEVEL_TONE[e.level]} className="mt-1.5" />
            <div className="min-w-0">
              <div className="text-sm">{e.title}</div>
              <div className="flex items-center gap-2 text-xs text-muted-foreground">
                {formatRelative(e.at)}
                <HostChip host={hosts.get(e.hostId ?? "")} />
                {e.stackId && (
                  <Link to={`/stacks/${e.stackId}`} className="text-primary-text hover:underline">
                    {t("openStack")}
                  </Link>
                )}
              </div>
            </div>
          </li>
        ))}
      </ol>
    </ScrollArea>
  );
}
