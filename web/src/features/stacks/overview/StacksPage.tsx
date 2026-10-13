import { AlertTriangle, Download, Layers, Play, Plus, RefreshCw, RotateCw, Square } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { stackStatus } from "@/domain/status";
import { summarizeUpdates } from "@/domain/updates";
import { BulkActionBar } from "@/shared/component-set/bulk-action-bar";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { useMetrics } from "@/data/live";
import { HostBlocks } from "@/shared/components/host-blocks";
import { LoadMetricToggle, LoadSortHead } from "@/shared/components/overview/load-sort";
import { OverviewTable, type OverviewColumn, type TableView } from "@/shared/components/overview/overview-table";
import { OverviewSkeleton } from "@/shared/components/overview-skeleton";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope, useVisibleContainers, useVisibleStacks } from "@/shared/hooks/use-host-scope";
import { useOverviewSelection } from "@/shared/hooks/use-overview-selection";
import { sortForMetric, type LoadMetric } from "@/shared/lib/load-sort";
import { nextSort, type SortDir } from "@/shared/lib/table-sort";
import { setPreference, usePreference } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Checkbox } from "@/shared/ui/checkbox";

import type { Stack } from "@/domain/model";
import { HiddenSection, type HiddenEntry } from "@/shared/components/hidden-items";
import { HostChip } from "@/shared/components/host-chip";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { splitHidden, useHiddenIds } from "@/shared/preferences/hidden-store";

import { StackStatusBadge } from "../StackStatusBadge";
import { allLabels, DEFAULT_STACK_SORT, filterStacks, sortStacks, stackSortDefaultDir, type StackSort, type StackSortKey, type StatusFilter } from "./stack-filters";
import { StackRow } from "./StackRow";
import { StacksToolbar } from "./StacksToolbar";

export default function StacksPage() {
  const t = useTranslations("stacks");
  const tc = useTranslations("common.host");
  const to = useTranslations("common.overview");
  const th = useTranslations("common.hidden");
  const stacks = useVisibleStacks();
  const containers = useVisibleContainers();
  const hosts = useHostScope();
  const mode = usePreference("stacksMode");
  const layout = usePreference("stacksLayout");
  const cmd = useStackCommands();
  const [query, setQuery] = useState("");
  const [status, setStatus] = useState<StatusFilter>("all");
  const [sort, setSort] = useState<StackSort>(DEFAULT_STACK_SORT);
  const [hostDir, setHostDir] = useState<SortDir>("asc");
  const [labels, setLabels] = useState<string[]>([]);
  const [metric, setMetric] = useState<LoadMetric>("cpu");
  const { selecting, selected, setSelected, onSelect, clear: clearSelection } = useOverviewSelection(mode);
  const onSort = (key: StackSortKey) => setSort((s) => nextSort(s, key, stackSortDefaultDir));
  const onMetric = (m: LoadMetric) => {
    setMetric(m);
    setSort((s) => sortForMetric(s, "load"));
  };
  // The page follows live values only while sorting by load.
  const snapshot = useMetrics((snap) => (sort.key === "load" ? snap : undefined));

  const hiddenIds = useHiddenIds("stacks");
  const { shown: rows, hidden } = useMemo(
    () => splitHidden(sortStacks(filterStacks(stacks.visible, hosts.byId, { query, status, labels }), hosts.byId, sort, { snapshot, metric }), hiddenIds),
    [stacks.visible, hosts.byId, query, status, labels, sort, snapshot, metric, hiddenIds],
  );
  const hiddenTotal = stacks.visible.filter((s) => hiddenIds.includes(s.id)).length;
  const hiddenEntries = (list: Stack[], withHost: boolean): HiddenEntry[] =>
    list.map((s) => ({
      id: s.id,
      name: s.name,
      open: (
        <Link to={`/stacks/${s.id}`} className="rounded-sm outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring" aria-label={th("open", { name: s.name })}>
          {s.name}
        </Link>
      ),
      meta: (
        <span className="flex flex-wrap items-center gap-1.5">
          {withHost && <HostChip host={hosts.byId.get(s.hostId)} />}
          <StackStatusBadge stack={s} host={hosts.byId.get(s.hostId)} />
        </span>
      ),
    }));
  const memLimits = useMemo(() => {
    const out = new Map<string, number>();
    for (const c of containers.visible) if (c.stackId) out.set(c.stackId, (out.get(c.stackId) ?? 0) + c.memLimitMb);
    return out;
  }, [containers.visible]);
  const labelOptions = useMemo(() => allLabels(stacks.visible), [stacks.visible]);
  const counts = useMemo(() => {
    const st = stacks.visible.map((s) => stackStatus(s, hosts.byId.get(s.hostId)));
    return {
      running: st.filter((s) => s === "running" || s === "partial").length,
      problems: st.filter((s) => s === "problem" || s === "unreachable").length,
      updates: stacks.visible.reduce((n, s) => n + summarizeUpdates(s.services).pending, 0),
    };
  }, [stacks.visible, hosts.byId]);

  const visibleSelected = selected.filter((id) => rows.some((r) => r.id === id));
  const selectAll = (list: { id: string }[], label: string) => {
    const all = list.length > 0 && list.every((r) => selected.includes(r.id));
    return (
      <Checkbox
        checked={all}
        onCheckedChange={(v) => setSelected((s) => (v === true ? [...new Set([...s, ...list.map((r) => r.id)])] : s.filter((id) => !list.some((r) => r.id === id))))}
        aria-label={label}
      />
    );
  };
  const filtersActive = !!query.trim() || status !== "all" || labels.length > 0;
  const bulk = (fn: (id: string) => unknown) => {
    visibleSelected.forEach((id) => void fn(id));
    setSelected([]);
  };

  const scope = hosts.filtered ? t("scopeSome", { hosts: hosts.visible.map((h) => h.name).join(", ") }) : t("scopeAll");
  const key = (k: StackSortKey) => ({ key: k, label: k === "load" ? to("loadOption", { metric: to(`metric.${metric}`) }) : t(`columns.${k}`) });
  const loadHead = <LoadSortHead loadKey="load" sort={sort} onSort={onSort} metric={metric} onMetric={onMetric} label={t("columns.load")} />;
  // Condensed merges secondary columns into fewer cells; every value stays visible and sortable.
  const columns =
    (inBlock: boolean) =>
    (view: TableView): OverviewColumn<StackSortKey>[] => {
      const condensed = view === "condensed";
      const out: OverviewColumn<StackSortKey>[] = [{ id: "name", label: t("columns.name"), sorts: condensed && !inBlock ? [key("name"), key("host")] : [key("name")], className: "min-w-44" }];
      if (condensed) out.push({ id: "state", label: t("columns.status"), sorts: [key("status"), key("services")] });
      else {
        out.push({ id: "status", label: t("columns.status"), sorts: [key("status")] });
        if (!inBlock) out.push({ id: "host", label: t("columns.host"), sorts: [key("host")] });
        out.push({ id: "services", label: t("columns.services"), sorts: [key("services")] });
      }
      out.push({ id: "updates", label: t("columns.updates"), sorts: [key("updates")] }, { id: "load", label: t("columns.load"), sorts: [key("load")], head: loadHead, wide: true });
      if (mode === "advanced") {
        if (condensed) out.push({ id: "details", label: to("details"), sorts: [key("auto"), key("deploy")], wide: true });
        else out.push({ id: "ports", label: t("columns.ports"), className: "max-w-48", wide: true }, { id: "auto", label: t("columns.auto"), sorts: [key("auto")] }, { id: "deploy", label: t("columns.deploy"), sorts: [key("deploy")] });
      }
      return out;
    };
  const fitKey = `${mode}|${selecting}|${metric}`;
  const headProps = { sort, onSort, sortLabel: (column: string) => t("sortBy", { column }), selecting, actionsLabel: t("columns.actions"), dense: mode === "advanced", sortExtra: <LoadMetricToggle metric={metric} onMetric={onMetric} /> };

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description", { scope })}
        actions={
          <Button asChild className="group">
            <Link to="/stacks/new">
              <Plus data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:rotate-90" />
              {t("newStack")}
            </Link>
          </Button>
        }
      />
      <StatStrip
        items={[
          { label: t("strip.stacks"), value: stacks.isPending ? "–" : stacks.visible.length, icon: <Layers /> },
          { label: t("strip.running"), value: stacks.isPending ? "–" : counts.running, icon: <Play />, tone: counts.running ? "ok" : undefined },
          { label: t("strip.problems"), value: stacks.isPending ? "–" : counts.problems, icon: <AlertTriangle />, tone: counts.problems ? "err" : undefined },
          { label: t("strip.updates"), value: stacks.isPending ? "–" : counts.updates, icon: <Download />, tone: counts.updates ? "warn" : undefined, sub: t("strip.updatesSub") },
        ]}
      />
      <Card className="gap-3 p-4">
        <StacksToolbar
          query={query}
          onQuery={setQuery}
          status={status}
          onStatus={setStatus}
          labels={labelOptions}
          selectedLabels={labels}
          onToggleLabel={(l) => setLabels((s) => (s.includes(l) ? s.filter((x) => x !== l) : [...s, l]))}
          onResetLabels={() => setLabels([])}
          mode={mode}
          onMode={(m) => setPreference("stacksMode", m)}
          layout={layout}
          onLayout={(l) => setPreference("stacksLayout", l)}
        />
        {stacks.isPending ? (
          <OverviewSkeleton mode={mode} />
        ) : stacks.isError ? (
          <QueryError onRetry={() => void stacks.refetch()} />
        ) : stacks.visible.length === 0 ? (
          <EmptyState
            icon={<Layers />}
            title={t("emptyAll.title")}
            description={t("emptyAll.description")}
            action={
              <Button asChild>
                <Link to="/stacks/new">{t("newStack")}</Link>
              </Button>
            }
          />
        ) : rows.length === 0 && hidden.length > 0 && layout !== "hosts" ? (
          <HiddenSection kind="stacks" entries={hiddenEntries(hidden, true)} />
        ) : rows.length === 0 && hidden.length === 0 ? (
          <EmptyState
            icon={<Layers />}
            title={t("empty.title")}
            description={t("empty.description")}
            action={
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setStatus("all");
                  setLabels([]);
                }}
              >
                {t("empty.reset")}
              </Button>
            }
          />
        ) : layout === "hosts" ? (
          <HostBlocks
            sortBar="header"
            hosts={hosts.visible}
            rows={rows}
            hostOf={(s) => s.hostId}
            hostDir={hostDir}
            onHostDir={setHostDir}
            hideEmpty={filtersActive}
            labels={{ region: t("blocks.region"), order: t("blocks.order"), asc: t("blocks.asc"), desc: t("blocks.desc"), empty: t("blocks.empty"), status: (h) => tc(h.status) }}
            summary={(h, own) => t("blocks.summary", { count: own.length, running: own.filter((s) => stackStatus(s, h) === "running").length })}
            minWidth={mode === "advanced" ? "46rem" : "36rem"}
            maxColumns={mode === "advanced" ? 2 : 3}
            maxBlockWidth={mode === "advanced" ? "84rem" : "56rem"}
            fitKey={fitKey}
            columns={columns(true)}
            hidden={hidden}
            renderHidden={(h, own) => <HiddenSection kind="stacks" entries={hiddenEntries(own, false)} title={th("titleHost", { host: h.name })} />}
            liveSort={sort.key === "load"}
            selectFor={(h, own) => selectAll(own, t("blocks.selectHost", { host: h.name }))}
            {...headProps}
          >
            {(h, own, view, cols) =>
              own.map((s) => (
                <StackRow key={s.id} stack={s} host={h} mode={mode} view={view} columns={cols} selecting={selecting} selected={selected.includes(s.id)} onSelect={onSelect} memLimitMb={memLimits.get(s.id) ?? 0} inBlock />
              ))
            }
          </HostBlocks>
        ) : (
          <>
            <OverviewTable fitKey={`${fitKey}|${rows.length}`} columns={columns(false)} select={selectAll(rows, t("select.all"))} listLabel={t("title")} liveSort={sort.key === "load"} {...headProps}>
              {(view, cols) =>
                rows.map((s) => (
                  <StackRow key={s.id} stack={s} host={hosts.byId.get(s.hostId)} mode={mode} view={view} columns={cols} selecting={selecting} selected={selected.includes(s.id)} onSelect={onSelect} memLimitMb={memLimits.get(s.id) ?? 0} />
                ))
              }
            </OverviewTable>
            <HiddenSection kind="stacks" entries={hiddenEntries(hidden, true)} />
          </>
        )}
        {!stacks.isPending && !stacks.isError && (
          <p className="px-1 text-xs text-muted-foreground">{t("footer", { shown: rows.length, total: stacks.visible.length, hidden: hiddenTotal })}</p>
        )}
      </Card>
      <BulkActionBar count={visibleSelected.length} selectedLabel={t("select.selected", { count: visibleSelected.length })} clearLabel={t("select.clear")} onClear={clearSelection}>
        <Button size="sm" variant="secondary" className="h-8" onClick={() => bulk((id) => cmd.run(id, "start"))}>
          <Play data-icon="inline-start" />
          {t("bulk.start")}
        </Button>
        <Button size="sm" variant="secondary" className="h-8" onClick={() => bulk((id) => cmd.run(id, "stop"))}>
          <Square data-icon="inline-start" />
          {t("bulk.stop")}
        </Button>
        <Button size="sm" variant="secondary" className="h-8" onClick={() => bulk((id) => cmd.run(id, "restart"))}>
          <RotateCw data-icon="inline-start" />
          {t("bulk.restart")}
        </Button>
        <Button size="sm" variant="secondary" className="h-8" onClick={() => bulk((id) => cmd.run(id, "redeploy"))}>
          <RefreshCw data-icon="inline-start" />
          {t("bulk.redeploy")}
        </Button>
        <Button size="sm" className="h-8" onClick={() => bulk((id) => cmd.installUpdates(id))}>
          <Download data-icon="inline-start" />
          {t("bulk.update")}
        </Button>
      </BulkActionBar>
    </>
  );
}
