// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

import { useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useHosts, useContainers } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { queryKeys } from "@/data/query-keys";
import { HISTORY_RANGES, type HistoryRange } from "@/domain/history";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { QueryError } from "@/shared/components/page";
import { CombinedWindow } from "./CombinedWindow";
import { disrupted, selectWindow, type WindowSelection } from "./participants";
export default function DisruptionsPage() {
  const t = useTranslations("disruptions"), adapter = useAdapter();
  const hosts = useHosts(), containers = useContainers();
  const [range, setRange] = useState<HistoryRange>("24h");
  const [selection, setSelection] = useState<WindowSelection>();
  const enabled = hosts.data ?? [];
  const queries = useQueries({ queries: enabled.map((h) => ({ queryKey: queryKeys.hostContainerHistory(h.id, range), queryFn: () => adapter.getHostContainerHistory(h.id, range) })) });
  const histories = queries.flatMap((q) => q.data ? [q.data] : []);
  const pending = hosts.isPending || queries.some((q) => q.isPending);
  return <div className="grid min-w-0 gap-5"><h1 className="text-2xl font-semibold">{t("title")}</h1><p>{t("description")}</p>
    <SegmentedToggle label={t("range")} value={range} options={HISTORY_RANGES.map((value) => ({ value, label: t(`ranges.${value}`) }))} onChange={(value) => { setRange(value); setSelection(undefined); }} />
    {hosts.isError && <QueryError error={hosts.error} onRetry={() => void hosts.refetch()} />}
    {containers.isError && <QueryError error={containers.error} onRetry={() => void containers.refetch()} compact />}
    {pending && <p role="status">{t("loading")}</p>}
    {enabled.map((host, i) => { const q = queries[i]!; return <section key={host.id} className="grid min-w-0 gap-2 rounded-xl border p-4"><h2 className="font-semibold">{host.name}</h2>
      {q.isError && <QueryError error={q.error} onRetry={() => void q.refetch()} compact />}
      {q.data?.truncated && <p role="status">{t("truncated")}</p>}
      {q.data?.containers.map((lane) => { const current = containers.data?.find((c) => c.hostId === host.id && c.name === lane.name); return <div key={lane.name} className="grid min-w-0 gap-1">
        {current ? <Link className="text-sm underline" to={`/containers/${encodeURIComponent(current.id)}`}>{lane.name}</Link> : <span>{lane.name}</span>}
        <div className="flex h-7 gap-px">{lane.buckets.map((bucket) => disrupted(bucket) ? <button type="button" disabled={pending || containers.isPending || containers.isError} key={bucket.start} data-tone={bucket.state === "down" ? "err" : "warn"} className="min-w-0 flex-1 rounded-sm bg-(--tone)/70 focus-visible:outline-2" aria-label={t("bucket", { container: lane.name, state: t(`states.${bucket.state}`), start: bucket.start, end: bucket.end })} onClick={() => setSelection(selectWindow(bucket, histories, enabled, containers.data ?? []))} /> : <span key={bucket.start} className="min-w-0 flex-1 rounded-sm bg-muted" />)}</div>
      </div>; })}
    </section>; })}
    {!pending && !hosts.isError && !queries.some((q) => q.isError) && !histories.some((h) => h.containers.length) && <p>{t("empty")}</p>}
    {selection && <CombinedWindow selection={selection} />}
  </div>;
}
