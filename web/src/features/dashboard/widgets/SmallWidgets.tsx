import { Archive, CalendarClock, ExternalLink, RefreshCw, ScanSearch, ShieldCheck } from "lucide-react";
import type { ReactNode } from "react";
import { Link, useNavigate } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useMetrics } from "@/data/live";
import { formatCountdown, formatGb, formatRelative } from "@/domain/format";
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
  const t = useTranslations("dashboard.capacity");
  const metrics = useMetrics((s) => s?.hosts);
  return (
    <ScrollArea className="h-full">
    <ul className="flex flex-col gap-3.5 pr-2">
      {hosts.map((h) => {
        const m = metrics?.[h.id];
        const used = ((m?.disk ?? 0) / 100) * h.diskTotalGb;
        const free = h.diskTotalGb - used;
        const days = m && m.diskGrowthGbPerDay > 0 ? Math.round(free / m.diskGrowthGbPerDay) : Infinity;
        const tone = days < 30 ? "err" : days < 90 ? "warn" : "ok";
        return (
          <li key={h.id} className="flex flex-col gap-1.5">
            <Meter label={h.name} value={m?.disk ?? 0} color={h.color} sub={t("used", { used: formatGb(used), total: formatGb(h.diskTotalGb) })} />
            <div className="flex items-center justify-between text-[11px]">
              <span data-tone={h.status !== "online" ? "gray" : tone} className="font-medium text-(--tone)">
                {h.status !== "online" ? t("offline") : Number.isFinite(days) && days < 365 ? t("days", { days }) : t("plenty")}
              </span>
              <span className="text-muted-foreground tabular">{t("growth", { gb: formatNumber(m?.diskGrowthGbPerDay ?? 0) })}</span>
            </div>
          </li>
        );
      })}
    </ul>
    </ScrollArea>
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
  const sorted = [...jobs].sort((a, b) => a.nextRunAt.localeCompare(b.nextRunAt));
  if (!sorted.length) return <p className="text-sm text-muted-foreground">{t("empty")}</p>;
  return (
    <ScrollArea className="h-full">
      <ol className="relative flex flex-col gap-1 pr-2 before:absolute before:top-3 before:bottom-3 before:left-[13px] before:w-px before:bg-border">
        {sorted.map((j) => {
          const when = formatCountdown(j.nextRunAt, now);
          const target = j.kind === "backup" && !j.volumeId ? "backupList" : j.kind === "auto-update" && !j.stackId ? "autoPolicies" : j.kind;
          return (
            <li key={j.id} className="relative">
              {/* A real link: it opens the page that configures this job, also in a new tab. */}
              <Link
                to={jobHref(j)}
                data-glow="row"
                data-job={j.kind}
                aria-label={t("open", { title: j.title, when, target: t(`targets.${target}`) })}
                className="group/job flex items-center gap-3 rounded-lg py-1.5 pr-1 outline-none hover:bg-muted/40 focus-visible:ring-[3px] focus-visible:ring-ring/50"
              >
                <IconTile tone={JOB_TONE[j.kind]} size="sm" className="z-10 ring-4 ring-card">
                  {JOB_ICON[j.kind]}
                </IconTile>
                <div className="min-w-0 flex-1">
                  <div className="truncate text-sm group-hover/job:underline group-hover/job:underline-offset-4">{j.title}</div>
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

const SEV: { key: Severity; tone: "err" | "orange" | "warn" | "lime" }[] = [
  { key: "critical", tone: "err" },
  { key: "high", tone: "orange" },
  { key: "medium", tone: "warn" },
  { key: "low", tone: "lime" },
];

export function SecurityWidget({ security, visibleHostIds }: { security: SecurityOverview; visibleHostIds: Set<string> }) {
  const t = useTranslations("dashboard.security");
  const navigate = useNavigate();
  const adapter = useAdapter();
  const scanCapability = useCapability("runScan");
  if (!security.enabled) {
    return (
      <div className="flex h-full flex-col items-start justify-center gap-3">
        <p className="text-sm text-muted-foreground">{t("disabled")}</p>
        <Button size="sm" variant="outline" disabled={!scanCapability.available} title={scanCapability.reason} onClick={() => void adapter.runScan()}>
          <ShieldCheck data-icon="inline-start" /> {t("enable")}
        </Button>
      </div>
    );
  }
  const images = security.images.filter((i) => visibleHostIds.has(i.hostId));
  const totals = SEV.map((s) => ({ ...s, n: images.reduce((n, i) => n + i.counts[s.key], 0) }));
  const runningCritical = images.filter((i) => i.usage === "running").reduce((n, i) => n + i.counts.critical, 0);
  const all = totals.reduce((n, s) => n + s.n, 0);
  return (
    <div className="flex h-full flex-col gap-4">
      <div className="grid grid-cols-4 gap-2">
        {totals.map((s) => (
          <div key={s.key} data-tone={s.tone} className="rounded-xl border border-(--tone)/25 bg-(--tone)/8 px-2 py-3 text-center">
            <div className="text-2xl font-semibold text-(--tone) tabular">{s.n}</div>
            <div className="text-[11px] text-muted-foreground">{t(`severities.${s.key}`)}</div>
          </div>
        ))}
      </div>
      <div className="flex h-2.5 gap-0.5 overflow-hidden rounded-full" aria-hidden>
        {totals.map((s) => (s.n ? <span key={s.key} data-tone={s.tone} className="rounded-full bg-(--tone)" style={{ flexGrow: s.n / Math.max(1, all) }} /> : null))}
      </div>
      <p data-tone={runningCritical ? "err" : "ok"} className="text-sm text-(--tone)">
        {t("running", { count: runningCritical })}
      </p>
      <Button size="sm" variant="outline" className="mt-auto self-start" onClick={() => navigate("/security")}>
        {t("open")}
      </Button>
    </div>
  );
}

export function AppsWidget({ stacks, hosts }: { stacks: Stack[]; hosts: Map<string, Host> }) {
  const t = useTranslations("dashboard.apps");
  const withUrl = stacks.filter((s) => s.url);
  if (!withUrl.length) return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  return (
    <div className="grid grid-cols-2 gap-2">
      {withUrl.map((s) => {
        const host = hosts.get(s.hostId);
        return (
          <button
            key={s.id}
            type="button"
            onClick={() => toast.info(t("demo", { url: s.url ?? "" }))}
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
          </button>
        );
      })}
    </div>
  );
}

const LEVEL_TONE: Record<EventLevel, "err" | "warn" | "ok" | "info"> = { error: "err", warn: "warn", ok: "ok", info: "info" };

export function EventsWidget({ events, hosts }: { events: OpsEvent[]; hosts: Map<string, Host> }) {
  const t = useTranslations("dashboard.events");
  if (!events.length) return <p className="text-sm text-muted-foreground">{t("empty")}</p>;
  return (
    <ScrollArea className="h-full">
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
