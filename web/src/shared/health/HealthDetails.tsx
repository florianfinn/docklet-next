// Source: docklet-next/issues/636
// Revision: 7a44b4b5c32ccc64b68e027a5496fab1d26e90e2
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 stack Health tab and reusable health details. M42 #637: shared hint text and one-sentence probe line.

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
export function HealthDetails({ containerId, historyRef, range = "24h" }: {
  containerId?: string; historyRef?: ContainerHistoryRef; range?: HistoryRange;
}) {
  const t = useTranslations("health");
  const health = useContainerHealth(containerId);
  const history = useContainerHistory(containerId ? historyRef : undefined, range);
  if (!containerId) return <p>{t("noContainer")}</p>;
  return <div className="grid gap-3 text-sm">
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
  return <>
    {hint && <p data-health-hint>{hint}</p>}
    {d.failingStreak > 0 && <p>{t("streak", { count: d.failingStreak })}</p>}
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
