import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { useCapability } from "@/data/use-capability";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Archive, Database, Eraser, FolderInput, HardDrive, PackageOpen, Plus, Search } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { Link, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useVolumes } from "@/data/queries";
import { formatBytes } from "@/domain/format";
import { filterByHosts } from "@/domain/hosts";
import { volumeLabel, type Volume } from "@/domain/volumes";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { HostBlocks } from "@/shared/components/host-blocks";
import { OverviewViewControls } from "@/shared/components/overview-mode-toggle";
import { OverviewTable, type OverviewColumn, type TableView } from "@/shared/components/overview/overview-table";
import { OverviewSkeleton } from "@/shared/components/overview-skeleton";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { nextSort, type SortDir } from "@/shared/lib/table-sort";
import { setPreference, useHostSelection, usePreference } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";

import { CreateVolumeDialog } from "./CreateVolumeDialog";
import { PruneVolumesDialog } from "./PruneVolumesDialog";
import { useVolumeCommands } from "./use-volume-commands";
import { DEFAULT_VOLUME_SORT, filterVolumes, sortVolumes, volumeSortDefaultDir, type KindFilter, type UsageFilter, type VolumeSort, type VolumeSortKey } from "./volume-filters";
import { VolumeRow } from "./VolumeRow";

export default function VolumesPage() {
  const t = useTranslations("volumes");
  const ts = useTranslations("stacks");
  const tc = useTranslations("common.host");
  const volumes = useVolumes();
  const createCapability = useCapability("createVolume");
  const pruneCapability = useCapability("planVolumePrune");
  const layout = usePreference("volumesLayout");
  const [sort, setSort] = useState<VolumeSort>(DEFAULT_VOLUME_SORT);
  const [hostDir, setHostDir] = useState<SortDir>("asc");
  const onSort = (key: VolumeSortKey) => setSort((s) => nextSort(s, key, volumeSortDefaultDir));
  const hosts = useHostScope();
  const selection = useHostSelection();
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  const [creating, setCreating] = useState(false);
  const [pruning, setPruning] = useState(false);
  const [removing, setRemoving] = useState<Volume | null>(null);
  const cmd = useVolumeCommands();
  const tm = useTranslations("volumes.menu");
  const onBackup = useCallback(
    async (v: Volume) => {
      if (await cmd.backup(v.id)) toast(tm("backupStarted", { name: volumeLabel(v) }));
    },
    [cmd, tm],
  );
  const kind = (params.get("kind") ?? "all") as KindFilter;
  const usage = (params.get("usage") ?? "all") as UsageFilter;
  const setParam = (key: string, value: string) =>
    setParams(
      (p) => {
        const next = new URLSearchParams(p);
        if (value === "all") next.delete(key);
        else next.set(key, value);
        return next;
      },
      { replace: true },
    );

  const visible = useMemo(() => filterByHosts(volumes.data ?? [], selection), [volumes.data, selection]);
  const rows = useMemo(() => sortVolumes(filterVolumes(visible, { query, kind, usage }), hosts.byId, sort), [visible, query, kind, usage, hosts.byId, sort]);
  const stats = useMemo(
    () => ({
      volumes: visible.filter((v) => v.kind === "volume").length,
      binds: visible.filter((v) => v.kind === "bind").length,
      unused: visible.filter((v) => !v.consumers.length).length,
      size: visible.reduce((n, v) => n + v.sizeBytes, 0),
      backup: visible.filter((v) => v.backup?.enabled).length,
    }),
    [visible],
  );
  const scope = hosts.filtered ? ts("scopeSome", { hosts: hosts.visible.map((h) => h.name).join(", ") }) : ts("scopeAll");
  const pending = volumes.isPending;
  const key = (k: VolumeSortKey) => ({ key: k, label: t(`columns.${k}`) });
  const columns =
    (inBlock: boolean) =>
    (view: TableView): OverviewColumn<VolumeSortKey>[] =>
      view === "condensed"
        ? [
            { id: "name", label: t("columns.name"), sorts: [key("name"), key("kind"), ...(inBlock ? [] : [key("host")])], className: "min-w-52" },
            { id: "consumers", label: t("columns.consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
            { id: "size", label: t("columns.size"), sorts: [key("size")] },
            { id: "backup", label: t("columns.backup"), sorts: [key("backup")] },
          ]
        : [
            { id: "name", label: t("columns.name"), sorts: [key("name")], className: "min-w-52" },
            { id: "kind", label: t("columns.kind"), sorts: [key("kind")] },
            ...(inBlock ? [] : [{ id: "host", label: t("columns.host"), sorts: [key("host")] }]),
            { id: "consumers", label: t("columns.consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
            { id: "size", label: t("columns.size"), sorts: [key("size")] },
            { id: "backup", label: t("columns.backup"), sorts: [key("backup")] },
          ];
  const headProps = { sort, onSort, sortLabel: (column: string) => ts("sortBy", { column }), selecting: false, actionsLabel: t("columns.actions"), dense: false };
  const filtersActive = !!query.trim() || kind !== "all" || usage !== "all";

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description", { scope })}
        actions={
          <>
            <Button asChild variant="outline">
              <Link to="/backups">
                <Archive data-icon="inline-start" />
                {t("backups.title")}
              </Link>
            </Button>
            <Button variant="outline" onClick={() => setPruning(true)} disabled={pending || !pruneCapability.available} title={pruneCapability.reason}>
              <Eraser data-icon="inline-start" />
              {t("prune.open")}
            </Button>
            <Button className="group" disabled={!createCapability.available} title={createCapability.reason} onClick={() => setCreating(true)}>
              <Plus data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:rotate-90" />
              {t("create")}
            </Button>
          </>
        }
      />
      <InventoryNotice hosts={hosts.visible} items={visible} />
      <StatStrip
        className="md:grid-cols-5"
        items={[
          { label: t("strip.volumes"), value: pending ? "–" : stats.volumes, icon: <Database /> },
          { label: t("strip.binds"), value: pending ? "–" : stats.binds, icon: <FolderInput /> },
          { label: t("strip.unused"), value: pending ? "–" : stats.unused, icon: <PackageOpen />, tone: stats.unused ? "warn" : undefined },
          { label: t("strip.size"), value: pending ? "–" : formatBytes(stats.size), icon: <HardDrive /> },
          { label: t("strip.backup"), value: pending ? "–" : `${stats.backup}/${visible.length}`, icon: <Archive />, tone: "ok" },
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
          <SegmentedToggle<KindFilter> value={kind} onChange={(v) => setParam("kind", v)} label={t("kindFilter")} options={(["all", "volume", "bind"] as const).map((k) => ({ value: k, label: t(`kinds.${k}`) }))} fit="content" wrap />
          <SegmentedToggle<UsageFilter> value={usage} onChange={(v) => setParam("usage", v)} label={t("usageFilter.label")} options={(["all", "used", "unused"] as const).map((k) => ({ value: k, label: t(`usageFilter.${k}`) }))} fit="content" wrap />
          <span className="flex-1" />
          <OverviewViewControls layout={layout} onLayout={(l) => setPreference("volumesLayout", l)} labels={{ layout: ts("layout.label"), table: ts("layout.table"), hosts: ts("layout.hosts"), tableHint: ts("layout.tableHint"), hostsHint: ts("layout.hostsHint") }} />
        </div>
        {pending ? (
          <OverviewSkeleton mode="compact" rows={8} />
        ) : volumes.isError ? (
          <QueryError onRetry={() => void volumes.refetch()} />
        ) : visible.length === 0 ? (
          <EmptyState icon={<Database />} title={t("emptyAll.title")} description={t("emptyAll.description")} />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={<Database />}
            title={t("empty.title")}
            action={
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setParams({}, { replace: true });
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
            hostOf={(v) => v.hostId}
            hostDir={hostDir}
            onHostDir={setHostDir}
            hideEmpty={filtersActive}
            labels={{ region: t("blocks.region"), order: ts("blocks.order"), asc: ts("blocks.asc"), desc: ts("blocks.desc"), empty: t("blocks.empty"), status: (h) => tc(h.status) }}
            summary={(_, own) => t("blocks.summary", { count: own.length, used: own.filter((v) => v.consumers.length > 0).length })}
            minWidth="40rem"
            fitKey="volumes"
            columns={columns(true)}
            selectFor={() => null}
            {...headProps}
          >
            {(h, own, view, cols) => own.map((v) => <VolumeRow key={v.id} volume={v} host={h} view={view} columns={cols} inBlock onRemove={setRemoving} onBackup={onBackup} />)}
          </HostBlocks>
        ) : (
          <OverviewTable fitKey={`volumes|${rows.length}`} columns={columns(false)} listLabel={t("title")} {...headProps}>
            {(view, cols) => rows.map((v) => <VolumeRow key={v.id} volume={v} host={hosts.byId.get(v.hostId)} view={view} columns={cols} onRemove={setRemoving} onBackup={onBackup} />)}
          </OverviewTable>
        )}
        {!pending && !volumes.isError && <p className="px-1 text-xs text-muted-foreground">{t("footer", { shown: rows.length, total: visible.length })}</p>}
      </Card>
      <PruneVolumesDialog open={pruning} onOpenChange={setPruning} hosts={hosts.visible.length ? hosts.visible : hosts.all} />
      <ConfirmDialog
        open={!!removing}
        onOpenChange={(o) => !o && setRemoving(null)}
        title={t("detail.deleteTitle", { name: removing ? volumeLabel(removing) : "" })}
        description={t("detail.deleteBody")}
        confirmLabel={t("detail.deleteConfirm")}
        destructive
        onConfirm={async () => {
          const v = removing;
          if (!v || !(await cmd.remove(v.id))) return false;
          toast.success(t("detail.deleted", { name: volumeLabel(v) }));
        }}
      />
      <CreateVolumeDialog open={creating} onOpenChange={setCreating} hosts={hosts.visible.length ? hosts.visible : hosts.all} volumes={volumes.data ?? []} />
    </>
  );
}
