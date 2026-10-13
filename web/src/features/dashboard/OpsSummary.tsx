import { Activity, CircleCheckBig, Download, ServerCog, ShieldAlert, Siren } from "lucide-react";
import { useState, type ReactNode } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { openActivity } from "@/features/activity/activity-store";
import type { ContainerInfo, Host, Stack, Tone } from "@/domain/model";
import { InteractiveHoverCard } from "@/shared/components/interactive-hover-card";
import { SimulationBadge } from "@/shared/components/simulation-badge";
import { LuminousIcon, ToneDot } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { DotField } from "@/shared/visual/DotField";

import type { AttentionItem } from "./attention";
import { fleetBreakdown, fleetUnits, problemCounts, problemItems, type FleetBreakdown, type FleetUnit } from "./fleet-status";
import { orderHosts, useFleetView, type HostAreaSize } from "./fleet-view-store";
import { FleetLine } from "./FleetLine";
import { FleetRing } from "./FleetRing";
import { HostCards } from "./HostCards";

function PulseChip({ tone, icon, children, to, onClick }: { tone: Tone; icon: ReactNode; children: ReactNode; to?: string; onClick?: () => void }) {
  const cls = cn(
    "lift group inline-flex h-8 items-center gap-1.5 rounded-lg border border-(--tone)/30 bg-(--tone)/10 px-2.5 text-xs font-medium text-(--tone) outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 [&_svg]:size-3.5",
  );
  const inner = (
    <>
      <span className="tilt">{icon}</span>
      {children}
    </>
  );
  return to ? (
    <Link to={to} data-tone={tone} className={cls}>
      {inner}
    </Link>
  ) : (
    <button type="button" data-tone={tone} className={cls} onClick={onClick}>
      {inner}
    </button>
  );
}

/** Segment colours with counts; the same states the ring and its popups use. */
function RingLegend({ b }: { b: FleetBreakdown }) {
  const t = useTranslations("dashboard.summary.legend");
  const rows: [keyof FleetBreakdown, Tone, boolean][] = [
    ["running", "ok", true],
    ["partial", "warn", b.partial > 0],
    ["problem", "err", b.problem > 0],
    ["unreachable", "err", b.unreachable > 0],
    ["starting", "info", b.starting > 0],
    ["stopped", "gray", b.stopped > 0],
    ["unknown", "orange", b.unknown > 0],
  ];
  return (
    <ul aria-label={t("label")} className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground" data-slot="ring-legend">
      {rows
        .filter(([, , show]) => show)
        .map(([key, tone]) => (
          <li key={key} className="inline-flex items-center gap-1.5">
            {key === "unreachable" ? (
              <span data-tone={tone} className="inline-block h-2 w-3.5 rounded-[2px] border-2 border-dashed border-(--tone)" aria-hidden />
            ) : (
              <ToneDot tone={tone} />
            )}
            {t(key as "running", { count: b[key] })}
          </li>
        ))}
    </ul>
  );
}

/** Problems chip with an honest breakdown: one per disconnected host, not per container on it. */
function ProblemsChip({ attention, breakdown }: { attention: AttentionItem[]; breakdown: FleetBreakdown }) {
  const t = useTranslations("dashboard.summary");
  const tb = useTranslations("dashboard.summary.breakdown");
  const counts = problemCounts(attention);
  const total = problemItems(attention).length;
  const tone: Tone = total ? "err" : "ok";
  return (
    <InteractiveHoverCard
      label={`${t("chips.problems", { count: total })}: ${tb("title")}`}
      contentClassName="w-80"
      triggerClassName="rounded-lg"
      content={
        <div className="text-xs">
          <div data-tone={tone} className="tone-surface flex items-center gap-2.5 p-3.5">
            <LuminousIcon tone={tone} className="size-8 [&_svg]:size-4">
              <Siren />
            </LuminousIcon>
            <span className="text-sm font-semibold text-(--tone)">{tb("title")}</span>
          </div>
          <ul className="flex flex-col gap-2 p-3.5">
            {total === 0 && <li className="text-muted-foreground">{tb("none")}</li>}
            {counts.offline ? (
              <li className="flex gap-2">
                <span data-tone="err" className="mt-1 inline-block h-2 w-3.5 shrink-0 rounded-[2px] border-2 border-dashed border-(--tone)" aria-hidden />
                <span>
                  <span className="font-medium">{tb("offline", { count: counts.offline })}</span>
                  <span className="block text-muted-foreground">{tb("offlineSub", { units: breakdown.unreachable, containers: breakdown.containersUnreachable })}</span>
                </span>
              </li>
            ) : null}
            {(["unhealthy", "failed", "disk"] as const).map((k) =>
              counts[k] ? (
                <li key={k} className="flex items-center gap-2">
                  <ToneDot tone="err" />
                  <span className="font-medium">{tb(k, { count: counts[k] ?? 0 })}</span>
                </li>
              ) : null,
            )}
            <li className="text-[11px] text-muted-foreground">{tb("separate")}</li>
          </ul>
          <div className="flex gap-2 border-t border-border/50 bg-muted/40 p-2.5">
            {counts.offline ? (
              <Button asChild size="sm" variant="secondary" className="flex-1">
                <Link to="/hosts">{tb("toHosts")}</Link>
              </Button>
            ) : null}
            <Button asChild size="sm" variant="secondary" className="flex-1">
              <Link to="/stacks">{tb("toStacks")}</Link>
            </Button>
          </div>
        </div>
      }
    >
      <span data-tone={tone} data-slot="problems-chip" className="lift inline-flex h-8 items-center gap-1.5 rounded-lg border border-(--tone)/30 bg-(--tone)/10 px-2.5 text-xs font-medium text-(--tone) [&_svg]:size-3.5">
        <Siren />
        {t("chips.problems", { count: total })}
      </span>
    </InteractiveHoverCard>
  );
}

interface OpsSummaryProps {
  hosts: Host[];
  allHostsCount: number;
  stacks: Stack[];
  containers: ContainerInfo[];
  attention: AttentionItem[];
  attentionText: (item: AttentionItem) => string;
  updates: number;
  critical: number;
  runningActions: number;
  editing: boolean;
  animate: boolean;
  duration: number;
}

/** Joint ring, one ring per host or status lines; all read the same stack units. */
function FleetVisual({ hosts, units, label, center }: { hosts: Host[]; units: FleetUnit[]; label: string; center: { value: string; caption: string } }) {
  const t = useTranslations("dashboard.ring");
  const view = useFleetView();
  if (view.mode === "line") return <FleetLine hosts={hosts} units={units} label={label} />;
  if (view.mode === "perHost") {
    const size = Math.round(Math.max(130, view.ringSize * 0.66));
    return (
      <div className="flex flex-wrap justify-center gap-4" data-slot="fleet-rings">
        {hosts.map((h) => {
          const own = units.filter((u) => u.hostId === h.id);
          const running = own.filter((u) => u.state === "running").length;
          return <FleetRing key={h.id} hosts={[h]} units={own} size={size} fan={view.fan} label={t("hostRingLabel", { host: h.name, running, total: own.length })} center={{ value: `${running}/${own.length}`, caption: h.name }} />;
        })}
      </div>
    );
  }
  return <FleetRing hosts={hosts} units={units} label={label} center={center} size={view.ringSize} fan={view.fan} />;
}

/** Shared all-host situation: fleet visual, verdict, pulse chips and per-host live cards. */
export function OpsSummary({ hosts: byName, allHostsCount, stacks, containers, attention, attentionText, updates, critical, runningActions, editing, animate, duration }: OpsSummaryProps) {
  const t = useTranslations("dashboard.summary");
  const tr = useTranslations("dashboard.ring");
  const view = useFleetView();
  const [areaPreview, setAreaPreview] = useState<HostAreaSize | null>(null);
  const area = areaPreview ?? view.hostArea;
  const hosts = orderHosts(byName, view.hostOrder);
  const units = fleetUnits(hosts, stacks, containers);
  const breakdown = fleetBreakdown(hosts, units);
  const running = breakdown.running + breakdown.starting;
  const problems = problemItems(attention).length;
  const online = hosts.filter((h) => h.status === "online").length;
  const calm = attention.length === 0;
  const top = attention[0];
  const verdictTone: Tone = calm ? "ok" : problems || attention.some((a) => a.tone === "err") ? "err" : "warn";

  const counts = { running, partial: breakdown.partial, problems: breakdown.problem, unreachable: breakdown.unreachable, stopped: breakdown.stopped };
  const verdict = (
    // The column grows with the row; the calm, nearly opaque plate inside only wraps the text, in both themes.
    <div className="flex min-w-[min(100%,20rem)] flex-[1_1_20rem]" data-slot="ops-verdict">
      <div
        className="flex w-fit max-w-full flex-col gap-3 rounded-xl border border-border/50 bg-card/85 p-3.5 shadow-sm supports-[backdrop-filter]:bg-card/75 supports-[backdrop-filter]:backdrop-blur-sm"
        data-slot="ops-verdict-plate"
      >
        <div className="flex flex-wrap items-center gap-2 text-xs font-medium tracking-wide text-muted-foreground uppercase">
          {t("title")} · {hosts.length === allHostsCount ? t("scopeAll", { count: allHostsCount }) : t("scopeSome", { shown: hosts.length, count: allHostsCount })}
          <SimulationBadge className="normal-case" />
        </div>
        <div className="flex items-start gap-3">
          <LuminousIcon tone={verdictTone} className={cn("size-11", !calm && "animate-pulse-ring")}>
            {calm ? <CircleCheckBig /> : <Siren />}
          </LuminousIcon>
          <div className="min-w-0">
            <h2 data-tone={verdictTone} className="text-xl font-semibold tracking-tight text-(--tone)">
              {calm ? t("calm") : t("attention", { count: attention.length })}
            </h2>
            <p className="text-sm text-muted-foreground">{calm ? t("calmSub", { updates }) : t("attentionSub", { first: top ? attentionText(top) : "" })}</p>
          </div>
        </div>
        <div className="flex flex-wrap gap-2">
          <ProblemsChip attention={attention} breakdown={breakdown} />
          <PulseChip tone={updates ? "warn" : "ok"} icon={<Download />} to="/stacks">
            {t("chips.updates", { count: updates })}
          </PulseChip>
          <PulseChip tone={online === hosts.length ? "ok" : "err"} icon={<ServerCog />} to="/hosts">
            {t("chips.hosts", { online, total: hosts.length })}
          </PulseChip>
          <PulseChip tone={critical ? "err" : "ok"} icon={<ShieldAlert />} to="/security">
            {t("chips.critical", { count: critical })}
          </PulseChip>
          {runningActions > 0 && (
            <PulseChip tone="accent" icon={<Activity className="animate-pulse" />} onClick={() => openActivity()}>
              {t("chips.running", { count: runningActions })}
            </PulseChip>
          )}
        </div>
        <RingLegend b={breakdown} />
        <p className="text-xs text-muted-foreground tabular-nums" data-slot="fleet-counts">
          {t("counts", { units: breakdown.units, running: breakdown.containersRunning, containers: breakdown.containers })}
        </p>
      </div>
    </div>
  );
  const visual = (
    <FleetVisual hosts={hosts} units={units} label={view.mode === "line" ? tr("lineLabel", counts) : t("ringLabel", counts)} center={{ value: `${running}/${units.length}`, caption: t("ringCaption") }} />
  );
  const right = view.placement === "right";

  return (
    // data-slot names the summary for tests and styles, so data-glow opts the card into the edge light.
    <Card className="dot-grid @container/ops relative mb-6 gap-5 overflow-hidden p-5" data-slot="ops-summary" data-glow data-fleet-mode={view.mode} data-placement={view.placement}>
      <DotField />
      <div aria-hidden className="pointer-events-none absolute -top-24 -left-24 size-72 rounded-full bg-highlight/12 blur-3xl" />
      <div aria-hidden className="pointer-events-none absolute -right-20 -bottom-28 size-72 rounded-full bg-info/10 blur-3xl" />
      {/* Beside the visual, the host area takes w of 12 columns; its height follows its own rows, not the ring. */}
      <div
        className={cn("relative grid grid-cols-1 gap-5", right && "@min-[76rem]/ops:grid-cols-[minmax(0,var(--visual-share))_minmax(0,var(--host-share))] @min-[76rem]/ops:items-start")}
        style={{ "--visual-share": `${12 - area.w}fr`, "--host-share": `${area.w}fr` } as React.CSSProperties}
        data-slot="ops-layout"
      >
        {/* Visual and verdict share a row while both fit; otherwise the verdict wraps below with a readable width. */}
        <div className={cn("flex min-w-0 gap-6", view.mode === "line" ? "flex-col" : "flex-wrap items-center justify-center")}>
          {view.mode === "line" ? (
            <>
              {verdict}
              {visual}
            </>
          ) : (
            <>
              <div className="flex max-w-full shrink-0 justify-center">{visual}</div>
              {verdict}
            </>
          )}
        </div>
        {/* Beside the visual the host area sits at the bottom of the summary, whatever the ring's height. */}
        <div className={cn("min-w-0", right && "@min-[76rem]/ops:self-end")} data-slot="host-cards-column">
          <HostCards hosts={hosts} stacks={stacks} containers={containers} placement={view.placement} area={area} onAreaPreview={setAreaPreview} editing={editing} animate={animate} duration={duration} />
        </div>
      </div>
    </Card>
  );
}
