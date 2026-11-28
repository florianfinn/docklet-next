// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details. M42 #637: shared hint text and one-sentence probe line. M47 #692: condition facts, latest failing probe and last log lines.

import type { RuntimeCondition } from "@/domain/model";
import { useLogRead } from "@/data/live";
import { Link } from "react-router";
import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";
import { useContainerHealth, useContainerHistory } from "@/data/queries";
import { diagnoseHealth, probeCommand, type ContainerHealthDetails } from "@/domain/health";
import type { ContainerHistoryRef, HistoryRange } from "@/domain/history";
import { formatDateTime } from "@/domain/format";
import { StatusStrip } from "@/shared/history/HistorySection";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { useHealthHint } from "./health-hint";

/** Only the health projection is rendered; missing identities never start a read. */
/** `logsHref` links to another page; `onOpenLogs` reveals logs shown on the same page instead. */
export function HealthDetails({ containerId, historyRef, range = "24h", condition, state, logsHref, onOpenLogs }: {
  containerId?: string; historyRef?: ContainerHistoryRef; range?: HistoryRange; condition?: RuntimeCondition; state?: string; logsHref?: string; onOpenLogs?: () => void;
}) {
  const t = useTranslations("health");
  const health = useContainerHealth(containerId);
  const history = useContainerHistory(containerId ? historyRef : undefined, range);
  if (!containerId) return <p>{t("noContainer")}</p>;
  return <div className="grid gap-3 text-sm">
    {condition && condition.containerId === containerId && <ConditionFacts condition={condition} />}
    {(["stopped", "exited", "dead", "restarting"].includes(state ?? "") || (condition?.containerId === containerId && condition?.condition === "down")) && <LastLines key={containerId} containerId={containerId} href={logsHref} onOpen={onOpenLogs} />}
    {health.isError ? <p role="alert">{t("unavailable")}</p> : health.data ? <ProbeDetails key={containerId} details={health.data} /> : <p>{t("loading")}</p>}
    {historyRef && (history.isError ? <p role="alert">{t("historyUnavailable")}</p> : history.data ? <StatusStrip buckets={history.data.buckets} range={range} /> : null)}
  </div>;
}

function ProbeDetails({ details: d }: { details: ContainerHealthDetails }) {
  const t = useTranslations("health");
  const [now, setNow] = useState(Date.now);
  useEffect(() => {
    const timer = setInterval(() => setNow(Date.now()), 30_000);
    return () => clearInterval(timer);
  }, []);
  const hint = useHealthHint()(diagnoseHealth(d, now));
  const duration = (ms: number) => ms === 0 ? t("default") : ms % 3_600_000 === 0 ? t("hours", { value: ms / 3_600_000 }) : ms % 60_000 === 0 ? t("minutes", { value: ms / 60_000 }) : t("seconds", { value: ms / 1000 });
  const check = d.check;
  const failed = d.status === "unhealthy" || d.failingStreak > 0 ? d.probes.filter((p) => p.exitCode !== 0).toSorted((a, b) => (b.start ?? "").localeCompare(a.start ?? ""))[0] : undefined;
  return <>
    {hint && <p data-health-hint>{hint}</p>}
    {d.failingStreak > 0 && <p>{t("streak", { count: d.failingStreak })}</p>}
    {failed && <div className="rounded border p-3"><p>{t("latestFailure")}</p><p>{t("probeLine", { time: failed.start ? formatDateTime(failed.start) : "–", code: failed.exitCode })}</p><pre className="whitespace-pre-wrap break-all text-xs">{failed.output}</pre></div>}
    <Collapsible>
      <CollapsibleTrigger asChild><Button variant="outline" size="sm">{t("raw")}</Button></CollapsibleTrigger>
      <CollapsibleContent className="mt-3 grid min-w-0 gap-3">
        {check && <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-1">
          <dt>{t("command")}</dt><dd className="break-all font-mono">{probeCommand(check)}</dd>
          <dt>{t("interval")}</dt><dd>{duration(check.intervalMs)}</dd>
          <dt>{t("timeout")}</dt><dd>{duration(check.timeoutMs)}</dd>
          <dt>{t("retries")}</dt><dd>{check.retries || t("default")}</dd>
          <dt>{t("startPeriod")}</dt><dd>{duration(check.startPeriodMs)}</dd>
        </dl>}
        {d.probes.length === 0 && <p>{t("noProbes")}</p>}
        {[...d.probes].reverse().map((probe, i) => <div key={i} className="min-w-0 rounded border p-3">
          <p>{t("probeLine", { time: probe.start ? formatDateTime(probe.start) : "–", code: probe.exitCode })}</p>
          <pre className="overflow-x-auto whitespace-pre-wrap break-all text-xs">{probe.output}</pre>
        </div>)}
      </CollapsibleContent>
    </Collapsible>
  </>;
}

function ConditionFacts({ condition: c }: { condition: RuntimeCondition }) {
  const t = useTranslations("health");
  const reasons = ["oom", "restart-loop", "exit", "health", "paused", "intent", "never-started", "operator-stop"] as const;
  return <div className="grid gap-1">
    {reasons.includes(c.reason as typeof reasons[number]) && <p>{t(`conditionReasons.${c.reason as typeof reasons[number]}`)}</p>}
    {(["oom", "restart-loop", "exit"].includes(c.reason) || !!c.exitCode || !!c.restarts10m) && <p>{t("exitCode", { code: c.exitCode ?? 0 })}</p>}
    {(c.condition !== "running" || !!c.reason) && <p>{t("restarts", { count: c.restarts10m ?? 0 })}</p>}
  </div>;
}
function LastLines({ containerId, href, onOpen }: { containerId: string; href?: string; onOpen?: () => void }) {
  const t = useTranslations("health");
  const read = useLogRead({ kind: "container", id: containerId }, { tail: 20, maxLines: 20, maxBytes: 64 * 1024 });
  return <section className="min-w-0"><h4>{t("lastLines")}</h4>
    {read.result?.reason === "unavailable" && <p>{t("logsUnavailable")} <Button size="sm" variant="outline" onClick={read.retry}>{t("retry")}</Button></p>}
    {read.result?.reason === "cap-bytes" && <p>{t("logsCutOff")}</p>}
    <pre className="max-h-64 overflow-auto whitespace-pre-wrap break-all text-xs">{read.lines.map((l) => l.message).join("\n")}</pre>
    {href ? <Link className="underline" to={href}>{t("openLogs")}</Link> : onOpen && <Button size="sm" variant="link" className="h-auto justify-self-start p-0" onClick={onOpen}>{t("openLogs")}</Button>}
  </section>;
}
