import { useCapability } from "@/data/use-capability";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { AlertTriangle, Box, Boxes, Play, RotateCw, Search, Square } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import type { ContainerInfo } from "@/domain/model";
import { containerRuntime, RUNTIME_TONE } from "@/domain/status";
import { HiddenSection, type HiddenEntry } from "@/shared/components/hidden-items";
import { HostChip } from "@/shared/components/host-chip";
import { ToneBadge } from "@/shared/components/tone";
import { splitHidden, useHiddenIds } from "@/shared/preferences/hidden-store";
import { BulkActionBar } from "@/shared/component-set/bulk-action-bar";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { useMetrics } from "@/data/live";
import { HostBlocks } from "@/shared/components/host-blocks";
import { LoadMetricToggle, LoadSortHead } from "@/shared/components/overview/load-sort";
import { OverviewTable, type OverviewColumn, type TableView } from "@/shared/components/overview/overview-table";
import { OverviewViewControls } from "@/shared/components/overview-mode-toggle";
import { OverviewSkeleton } from "@/shared/components/overview-skeleton";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useContainerCommands } from "@/shared/hooks/use-container-commands";
import { useHostScope, useVisibleContainers } from "@/shared/hooks/use-host-scope";
import { useOverviewSelection } from "@/shared/hooks/use-overview-selection";
import { sortForMetric, type LoadMetric } from "@/shared/lib/load-sort";
import { nextSort, type SortDir } from "@/shared/lib/table-sort";
import { setPreference, usePreference } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { Checkbox } from "@/shared/ui/checkbox";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";

import { availableOrigins, containerSortDefaultDir, DEFAULT_CONTAINER_SORT, filterContainers, sortContainers, type ContainerSort, type ContainerSortKey, type OriginFilter } from "./container-filters";
import { ContainerInspector } from "./ContainerInspector";
import { ContainerRow } from "./ContainerRow";
import { useContainerRestart } from "./container-restart";
import { RecreateDialog } from "./RecreateDialog";

export default function ContainersPage() {
  const t = useTranslations("containers");
  const ts = useTranslations("stacks");
  const to = useTranslations("common.overview");
  const th = useTranslations("common.hidden");
  const tst = useTranslations("common.status");
  const containers = useVisibleContainers();
  const hosts = useHostScope();
  const cmd = useContainerCommands();
  const command = useCapability("runContainerCommand");
  const restart = useContainerRestart();
  const mode = usePreference("containersMode");
  const layout = usePreference("containersLayout");
  const tc = useTranslations("common.host");
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  // Links from the dashboard can preselect an origin (`?origin=standalone`); the choice then stays local.
  const [origin, setOrigin] = useState<OriginFilter>(() => {
    const o = params.get("origin");
    return o === "compose" || o === "standalone" || o === "unraid" || o === "unknown" ? o : "all";
  });
  const [sort, setSort] = useState<ContainerSort>(DEFAULT_CONTAINER_SORT);
  const [hostDir, setHostDir] = useState<SortDir>("asc");
  const [metric, setMetric] = useState<LoadMetric>("cpu");
  const onSort = (key: ContainerSortKey) => setSort((s) => nextSort(s, key, containerSortDefaultDir));
  const onMetric = (m: LoadMetric) => {
    setMetric(m);
    setSort((s) => sortForMetric(s, "load"));
  };
  // The page follows live values only while sorting by load.
  const snapshot = useMetrics((snap) => (sort.key === "load" ? snap : undefined));
  const { selecting, selected, setSelected, onSelect, clear: clearSelection } = useOverviewSelection(mode);
  // A snapshot keeps the dialog and its run alive after the old ID leaves the list.
  const [editing, setEditing] = useState<ContainerInfo | null>(null);
  const openId = params.get("c");

  // Origin filters exist only for origins present on the selected hosts; a search never removes them.
  const origins = useMemo(() => availableOrigins(containers.visible), [containers.visible]);
  if (origin !== "all" && containers.data && !origins.includes(origin)) setOrigin("all");
  const hiddenIds = useHiddenIds("containers");
  const { shown: rows, hidden } = useMemo(
    () => splitHidden(sortContainers(filterContainers(containers.visible, { query, origin }), hosts.byId, sort, { snapshot, metric }), hiddenIds),
    [containers.visible, origin, query, hosts.byId, sort, snapshot, metric, hiddenIds],
  );
  const hiddenTotal = containers.visible.filter((c) => hiddenIds.includes(c.id)).length;

  const counts = useMemo(() => {
    const st = containers.visible.map((c) => containerRuntime(c, hosts.byId.get(c.hostId)));
    return {
      running: st.filter((s) => s === "running").length,
      problems: st.filter((s) => s === "unhealthy" || s === "restarting").length,
      standalone: containers.visible.filter((c) => c.origin !== "compose").length,
    };
  }, [containers.visible, hosts.byId]);

  const open = useCallback((id: string) => setParams((p) => ({ ...Object.fromEntries(p), c: id })), [setParams]);
  const editById = useCallback((id: string) => setEditing(containers.data?.find((c) => c.id === id) ?? null), [containers.data]);
  const close = () =>
    setParams((p) => {
      const next = new URLSearchParams(p);
      next.delete("c");
      return next;
    });
  const hiddenEntries = (list: ContainerInfo[], withHost: boolean): HiddenEntry[] =>
    list.map((c) => ({
      id: c.id,
      name: c.name,
      open: (
        <button type="button" onClick={() => open(c.id)} className="rounded-sm text-left outline-none hover:underline focus-visible:ring-2 focus-visible:ring-ring" aria-label={th("open", { name: c.name })}>
          {c.name}
        </button>
      ),
      meta: (
        <span className="flex flex-wrap items-center gap-1.5">
          {withHost && <HostChip host={hosts.byId.get(c.hostId)} />}
          <ToneBadge tone={RUNTIME_TONE[containerRuntime(c, hosts.byId.get(c.hostId))]}>{tst(containerRuntime(c, hosts.byId.get(c.hostId)))}</ToneBadge>
        </span>
      ),
    }));
  const visibleSelected = selected.filter((id) => rows.some((r) => r.id === id));
  const selectAll = (list: { id: string }[], label: string) => (
    <Checkbox
      checked={list.length > 0 && list.every((r) => selected.includes(r.id))}
      onCheckedChange={(v) => setSelected((s) => (v === true ? [...new Set([...s, ...list.map((r) => r.id)])] : s.filter((id) => !list.some((r) => r.id === id))))}
      aria-label={label}
    />
  );
  const bulk = (command: "start" | "stop" | "restart") => {
    if (command === "restart") restart(containers.data?.filter((c) => visibleSelected.includes(c.id)) ?? []);
    else visibleSelected.forEach((id) => void cmd.run(id, command));
    setSelected([]);
  };
  const scope = hosts.filtered ? ts("scopeSome", { hosts: hosts.visible.map((h) => h.name).join(", ") }) : ts("scopeAll");
  const current = containers.visible.find((c) => c.id === openId) ?? containers.data?.find((c) => c.id === openId);
  const key = (k: ContainerSortKey) => ({ key: k, label: k === "load" ? to("loadOption", { metric: to(`metric.${metric}`) }) : t(`columns.${k}`) });
  const loadHead = <LoadSortHead loadKey="load" sort={sort} onSort={onSort} metric={metric} onMetric={onMetric} label={t("columns.load")} />;
  // Condensed merges secondary columns into fewer cells; every value stays visible and sortable.
  const columns =
    (inBlock: boolean) =>
    (view: TableView): OverviewColumn<ContainerSortKey>[] => {
      const condensed = view === "condensed";
      const advanced = mode === "advanced";
      const out: OverviewColumn<ContainerSortKey>[] = [{ id: "name", label: t("columns.name"), sorts: [key("name"), ...(condensed && !inBlock ? [key("host")] : []), ...(condensed && advanced ? [key("version")] : [])], className: "min-w-44" }];
      if (condensed) out.push({ id: "state", label: t("columns.status"), sorts: [key("status"), key("origin")] });
      else {
        out.push({ id: "status", label: t("columns.status"), sorts: [key("status")] });
        if (!inBlock) out.push({ id: "host", label: t("columns.host"), sorts: [key("host")] });
        out.push({ id: "origin", label: t("columns.origin"), sorts: [key("origin")] });
        if (advanced) out.push({ id: "version", label: t("columns.version"), sorts: [key("version")], className: "max-w-40" });
      }
      out.push({ id: "update", label: t("columns.update"), sorts: [key("update")], wide: true }, { id: "load", label: t("columns.load"), sorts: [key("load")], head: loadHead, wide: true });
      if (advanced) {
        if (condensed) out.push({ id: "details", label: to("details"), sorts: [key("restart"), key("uptime")], wide: true });
        else out.push({ id: "ports", label: t("columns.ports"), className: "max-w-40", wide: true }, { id: "networks", label: t("columns.networks"), className: "max-w-40", wide: true }, { id: "restart", label: t("columns.restart"), sorts: [key("restart")] }, { id: "uptime", label: t("columns.uptime"), sorts: [key("uptime")] });
      }
      return out;
    };
  const fitKey = `${mode}|${selecting}|${metric}`;
  const headProps = { sort, onSort, sortLabel: (column: string) => t("sortBy", { column }), selecting, actionsLabel: t("columns.actions"), dense: mode === "advanced", sortExtra: <LoadMetricToggle metric={metric} onMetric={onMetric} /> };

  return (
    <>
      <PageHeader title={t("title")} description={t("description", { scope })} />
      <InventoryNotice hosts={hosts.visible} items={containers.visible} />
      <StatStrip
        items={[
          { label: t("strip.total"), value: containers.isPending ? "–" : containers.visible.length, icon: <Boxes /> },
          { label: t("strip.running"), value: containers.isPending ? "–" : counts.running, icon: <Play />, tone: "ok" },
          { label: t("strip.unhealthy"), value: containers.isPending ? "–" : counts.problems, icon: <AlertTriangle />, tone: counts.problems ? "err" : undefined },
          { label: t("strip.standalone"), value: containers.isPending ? "–" : counts.standalone, icon: <Box /> },
        ]}
      />
      <Card className="gap-3 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <InputGroup className="w-full sm:w-72">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput type="search" placeholder={t("search")} aria-label={t("searchLabel")} value={query} onChange={(e) => setQuery(e.target.value)} />
          </InputGroup>
          <SegmentedToggle<OriginFilter> value={origin} onChange={setOrigin} label={t("originFilter")} options={(["all", ...origins] as const).map((o) => ({ value: o, label: t(`origins.${o}`) }))} fit="content" wrap />
          <span className="flex-1" />
          <OverviewViewControls
            mode={mode}
            onMode={(m) => setPreference("containersMode", m)}
            layout={layout}
            onLayout={(l) => setPreference("containersLayout", l)}
            labels={{
              label: ts("mode.label"),
              compact: ts("mode.compact"),
              advanced: ts("mode.advanced"),
              compactHint: ts("mode.compactHint"),
              advancedHint: ts("mode.advancedHint"),
              layout: ts("layout.label"),
              table: ts("layout.table"),
              hosts: ts("layout.hosts"),
              tableHint: ts("layout.tableHint"),
              hostsHint: ts("layout.hostsHint"),
            }}
          />
        </div>
        {containers.isPending ? (
          <OverviewSkeleton mode={mode} rows={9} />
        ) : containers.isError ? (
          <QueryError error={containers.error} onRetry={() => void containers.refetch()} />
        ) : containers.visible.length === 0 ? (
          <EmptyState icon={<Boxes />} title={t("emptyAll.title")} description={t("emptyAll.description")} />
        ) : rows.length === 0 && hidden.length > 0 && layout !== "hosts" ? (
          <HiddenSection kind="containers" entries={hiddenEntries(hidden, true)} />
        ) : rows.length === 0 && hidden.length === 0 ? (
          <EmptyState
            icon={<Boxes />}
            title={t("empty.title")}
            action={
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setOrigin("all");
                }}
              >
                {t("empty.reset")}
              </Button>
            }
          />
        ) : layout === "hosts" ? (
          <HostBlocks
            hosts={hosts.visible}
            rows={rows}
            hostOf={(c) => c.hostId}
            hostDir={hostDir}
            onHostDir={setHostDir}
            hideEmpty={!!query.trim() || origin !== "all"}
            labels={{ region: t("blocks.region"), order: ts("blocks.order"), asc: ts("blocks.asc"), desc: ts("blocks.desc"), empty: t("blocks.empty"), status: (h) => tc(h.status) }}
            summary={(h, own) => t("blocks.summary", { count: own.length, running: own.filter((c) => containerRuntime(c, h) === "running").length })}
            minWidth={mode === "advanced" ? "58rem" : "44rem"}
            fitKey={fitKey}
            columns={columns(true)}
            hidden={hidden}
            renderHidden={(h, own) => <HiddenSection kind="containers" entries={hiddenEntries(own, false)} title={th("titleHost", { host: h.name })} />}
            liveSort={sort.key === "load"}
            selectFor={(h, own) => selectAll(own, t("blocks.selectHost", { host: h.name }))}
            {...headProps}
          >
            {(h, own, view, cols) =>
              own.map((c) => <ContainerRow key={c.id} container={c} host={h} mode={mode} view={view} columns={cols} selecting={selecting} selected={selected.includes(c.id)} onSelect={onSelect} onOpen={open} onEdit={editById} inBlock />)
            }
          </HostBlocks>
        ) : (
          <>
            <OverviewTable fitKey={`${fitKey}|${rows.length}`} columns={columns(false)} select={selectAll(rows, t("select.all"))} listLabel={t("title")} liveSort={sort.key === "load"} {...headProps}>
              {(view, cols) =>
                rows.map((c) => (
                  <ContainerRow key={c.id} container={c} host={hosts.byId.get(c.hostId)} mode={mode} view={view} columns={cols} selecting={selecting} selected={selected.includes(c.id)} onSelect={onSelect} onOpen={open} onEdit={editById} />
                ))
              }
            </OverviewTable>
            <HiddenSection kind="containers" entries={hiddenEntries(hidden, true)} />
          </>
        )}
        {!containers.isPending && !containers.isError && <p className="px-1 text-xs text-muted-foreground">{t("footer", { shown: rows.length, total: containers.visible.length, hidden: hiddenTotal })}</p>}
      </Card>
      <ContainerInspector container={current} host={current ? hosts.byId.get(current.hostId) : undefined} onClose={close} />
      <RecreateDialog container={editing ?? undefined} open={!!editing} onOpenChange={(o) => !o && setEditing(null)} />
      <BulkActionBar count={visibleSelected.length} selectedLabel={t("select.selected", { count: visibleSelected.length })} clearLabel={t("select.clear")} onClear={clearSelection}>
        <Button size="sm" variant="secondary" className="h-8" disabled={!command.available || containers.visible.some((item) => visibleSelected.includes(item.id) && item.inventory?.stale)} title={command.reason} onClick={() => bulk("start")}>
          <Play data-icon="inline-start" /> {t("bulk.start")}
        </Button>
        <Button size="sm" variant="secondary" className="h-8" disabled={!command.available || containers.visible.some((item) => visibleSelected.includes(item.id) && item.inventory?.stale)} title={command.reason} onClick={() => bulk("stop")}>
          <Square data-icon="inline-start" /> {t("bulk.stop")}
        </Button>
        <Button size="sm" variant="secondary" className="h-8" disabled={!command.available || containers.visible.some((item) => visibleSelected.includes(item.id) && item.inventory?.stale)} title={command.reason} onClick={() => bulk("restart")}>
          <RotateCw data-icon="inline-start" /> {t("bulk.restart")}
        </Button>
      </BulkActionBar>
    </>
  );
}
