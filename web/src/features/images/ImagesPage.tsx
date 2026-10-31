import { isHostAvailable } from "@/domain/hosts";
import { useImageHostCapabilities } from "./use-image-capabilities";
import { InventoryNotice } from "@/features/stacks/InventoryNotice";
import { useCapability } from "@/data/use-capability";
import { Download, Eraser, HardDrive, Package, PackageOpen, Search, Unlink } from "lucide-react";
import { useCallback, useMemo, useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";

import { useImages } from "@/data/queries";
import { formatMem } from "@/domain/format";
import { filterByHosts } from "@/domain/hosts";
import { imageUsage, totalSizeMb, type DockerImage } from "@/domain/images";
import { hasUpdate } from "@/domain/updates";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { RemoveImageDialog } from "./RemoveImageDialog";
import { HostBlocks } from "@/shared/components/host-blocks";
import { OverviewViewControls } from "@/shared/components/overview-mode-toggle";
import { OverviewSkeleton } from "@/shared/components/overview-skeleton";
import { OverviewTable, type OverviewColumn, type TableView } from "@/shared/components/overview/overview-table";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { nextSort, type SortDir } from "@/shared/lib/table-sort";
import { setPreference, useHostSelection, usePreference } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/shared/ui/input-group";

import { PruneDialog, PullDialog } from "./ImageDialogs";
import { ImageDetails } from "./ImageDetails";
import { DEFAULT_IMAGE_SORT, filterImages, imageSortDefaultDir, sortImages, type ImageSort, type ImageSortKey, type UsageFilter } from "./image-filters";
import { ImageRow } from "./ImageRow";

const USAGES = ["all", "in-use", "unused", "dangling", "unknown"] as const;

/**
 * Images per host from the adapter, with consumers derived from containers.
 * Usage filter and the open image live in the URL (`?usage=`, `?image=`), so
 * links from containers and the security page land on the right entry.
 */
export default function ImagesPage() {
  const t = useTranslations("images");
  const ts = useTranslations("stacks");
  const tc = useTranslations("common.host");
  const images = useImages();
  const createCapability = useCapability("pullImage");
  const pruneCapability = useCapability("pruneImages");
  const hosts = useHostScope();
  const selection = useHostSelection();
  const layout = usePreference("imagesLayout");
  const [params, setParams] = useSearchParams();
  const [query, setQuery] = useState("");
  const [sort, setSort] = useState<ImageSort>(DEFAULT_IMAGE_SORT);
  const [hostDir, setHostDir] = useState<SortDir>("asc");
  const [pulling, setPulling] = useState<{ hostId: string; reference: string } | null>(null);
  const [pruning, setPruning] = useState(false);
  const [removing, setRemoving] = useState<DockerImage | null>(null);
  const onSort = (key: ImageSortKey) => setSort((s) => nextSort(s, key, imageSortDefaultDir));
  const usage = (USAGES as readonly string[]).includes(params.get("usage") ?? "") ? (params.get("usage") as UsageFilter) : "all";
  const openId = params.get("image");
  const setParam = useCallback(
    (key: string, value: string | null) =>
      setParams(
        (p) => {
          const next = new URLSearchParams(p);
          if (value === null || value === "all") next.delete(key);
          else next.set(key, value);
          return next;
        },
        { replace: key === "usage" },
      ),
    [setParams],
  );
  const open = useCallback((id: string) => setParam("image", id), [setParam]);

  const visible = useMemo(() => filterByHosts(images.data ?? [], selection), [images.data, selection]);
  const rows = useMemo(() => sortImages(filterImages(visible, { query, usage }), hosts.byId, sort), [visible, query, usage, hosts.byId, sort]);
  const stats = useMemo(() => {
    const unused = visible.filter((i) => imageUsage(i) === "unused");
    const dangling = visible.filter((i) => imageUsage(i) === "dangling");
    return {
      size: totalSizeMb(visible),
      unused: unused.length,
      unusedSize: totalSizeMb(unused),
      dangling: dangling.length,
      danglingSize: totalSizeMb(dangling),
      updates: visible.filter((i) => i.updateObservation ? ["version", "rebuild"].includes(i.updateObservation.state) : i.update && hasUpdate(i.update)).length,
    };
  }, [visible]);
  const current = (images.data ?? []).find((i) => i.id === openId);
  const scope = hosts.filtered ? ts("scopeSome", { hosts: hosts.visible.map((h) => h.name).join(", ") }) : ts("scopeAll");
  const pending = images.isPending;
  const key = (k: ImageSortKey) => ({ key: k, label: t(`columns.${k}`) });
  const columns =
    (inBlock: boolean) =>
    (view: TableView): OverviewColumn<ImageSortKey>[] =>
      view === "condensed"
        ? [
            { id: "name", label: t("columns.name"), sorts: [key("name"), key("version"), key("usage"), ...(inBlock ? [] : [key("host")])], className: "min-w-52" },
            { id: "consumers", label: t("columns.consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
            { id: "meta", label: t("columns.size"), sorts: [key("size"), key("created")] },
            { id: "update", label: t("columns.update"), sorts: [key("update")], wide: true },
          ]
        : [
            { id: "name", label: t("columns.name"), sorts: [key("name")], className: "min-w-52" },
            { id: "version", label: t("columns.version"), sorts: [key("version")] },
            ...(inBlock ? [] : [{ id: "host", label: t("columns.host"), sorts: [key("host")] }]),
            { id: "usage", label: t("columns.usage"), sorts: [key("usage")] },
            { id: "consumers", label: t("columns.consumers"), sorts: [key("consumers")], className: "max-w-72", wide: true },
            { id: "size", label: t("columns.size"), sorts: [key("size")] },
            { id: "created", label: t("columns.created"), sorts: [key("created")] },
            { id: "update", label: t("columns.update"), sorts: [key("update")], wide: true },
            { id: "security", label: t("columns.security") },
          ];
  const headProps = { sort, onSort, sortLabel: (column: string) => t("sortBy", { column }), selecting: false, actionsLabel: t("columns.actions"), dense: false };
  const filtersActive = !!query.trim() || usage !== "all";
  const pull = (img: DockerImage) => {
    if (img.tag !== null) setPulling({ hostId: img.hostId, reference: `${img.repository}:${img.tag}` });
  };
  const scopeHosts = hosts.visible.length ? hosts.visible : hosts.all;
  const scopedPull = useImageHostCapabilities("pullImage", scopeHosts.map((h) => h.id));
  const initialPullHost = scopeHosts.find((h) => isHostAvailable(h) && scopedPull.get(h.id));

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description", { scope })}
        actions={
          <>
            <Button variant="outline" onClick={() => setPruning(true)} disabled={pending || !pruneCapability.available} title={pruneCapability.reason}>
              <Eraser data-icon="inline-start" />
              {t("prune")}
            </Button>
            <Button disabled={!createCapability.available || !initialPullHost} title={createCapability.reason} onClick={() => setPulling({ hostId: initialPullHost?.id ?? "", reference: "" })}>
              <Download data-icon="inline-start" />
              {t("pull")}
            </Button>
          </>
        }
      />
      <InventoryNotice hosts={hosts.visible} items={visible} />
      <StatStrip
        className="md:grid-cols-5"
        items={[
          { label: t("strip.images"), value: pending ? "–" : visible.length, icon: <Package /> },
          { label: t("strip.size"), value: pending ? "–" : formatMem(stats.size), icon: <HardDrive /> },
          { label: t("strip.unused"), value: pending ? "–" : stats.unused, icon: <PackageOpen />, tone: stats.unused ? "warn" : undefined, sub: pending ? undefined : formatMem(stats.unusedSize) },
          { label: t("strip.dangling"), value: pending ? "–" : stats.dangling, icon: <Unlink />, tone: stats.dangling ? "orange" : undefined, sub: pending ? undefined : formatMem(stats.danglingSize) },
          { label: t("strip.updates"), value: pending ? "–" : stats.updates, icon: <Download />, tone: stats.updates ? "warn" : undefined },
        ]}
      />
      <Card className="gap-3 p-4">
        <div className="flex flex-wrap items-center gap-2">
          <InputGroup className="w-full sm:w-80">
            <InputGroupAddon>
              <Search />
            </InputGroupAddon>
            <InputGroupInput type="search" placeholder={t("search")} aria-label={t("searchLabel")} value={query} onChange={(e) => setQuery(e.target.value)} />
          </InputGroup>
          <SegmentedToggle<UsageFilter> value={usage} onChange={(v) => setParam("usage", v)} label={t("usageFilter.label")} options={USAGES.map((u) => ({ value: u, label: t(`usageFilter.${u}`) }))} fit="content" wrap />
          <span className="flex-1" />
          <OverviewViewControls layout={layout} onLayout={(l) => setPreference("imagesLayout", l)} labels={{ layout: ts("layout.label"), table: ts("layout.table"), hosts: ts("layout.hosts"), tableHint: ts("layout.tableHint"), hostsHint: ts("layout.hostsHint") }} />
        </div>
        {pending ? (
          <OverviewSkeleton mode="compact" rows={8} />
        ) : images.isError ? (
          <QueryError error={images.error} onRetry={() => void images.refetch()} />
        ) : visible.length === 0 ? (
          <EmptyState icon={<Package />} title={t("emptyAll.title")} description={t("emptyAll.description")} />
        ) : rows.length === 0 ? (
          <EmptyState
            icon={<Package />}
            title={t("empty.title")}
            action={
              <Button
                variant="outline"
                onClick={() => {
                  setQuery("");
                  setParam("usage", null);
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
            hostOf={(i) => i.hostId}
            hostDir={hostDir}
            onHostDir={setHostDir}
            hideEmpty={filtersActive}
            labels={{ region: t("blocks.region"), order: ts("blocks.order"), asc: ts("blocks.asc"), desc: ts("blocks.desc"), empty: t("blocks.empty"), status: (h) => tc(h.status) }}
            summary={(_, own) => t("blocks.summary", { count: own.length, size: formatMem(totalSizeMb(own)) })}
            minWidth="44rem"
            fitKey="images"
            columns={columns(true)}
            selectFor={() => null}
            {...headProps}
          >
            {(h, own, view, cols) => own.map((i) => <ImageRow key={i.id} image={i} host={h} view={view} columns={cols} inBlock onOpen={open} onPull={pull} onRemove={setRemoving} />)}
          </HostBlocks>
        ) : (
          <OverviewTable fitKey={`images|${rows.length}`} columns={columns(false)} listLabel={t("title")} {...headProps}>
            {(view, cols) => rows.map((i) => <ImageRow key={i.id} image={i} host={hosts.byId.get(i.hostId)} view={view} columns={cols} onOpen={open} onPull={pull} onRemove={setRemoving} />)}
          </OverviewTable>
        )}
        {!pending && !images.isError && <p className="px-1 text-xs text-muted-foreground">{t("footer", { shown: rows.length, total: visible.length })}</p>}
      </Card>
      <ImageDetails id={openId} image={current} host={current ? hosts.byId.get(current.hostId) : undefined} onClose={() => setParam("image", null)} onPull={pull} onRemove={setRemoving} />
      <PullDialog open={!!pulling} onOpenChange={(o) => !o && setPulling(null)} hosts={scopeHosts} images={images.data ?? []} initial={pulling ?? undefined} />
      <PruneDialog key={scopeHosts.map((h) => h.id).sort().join("|")} open={pruning} onOpenChange={setPruning} hosts={scopeHosts} images={images.data ?? []} />
      {removing && <RemoveImageDialog key={removing.id} image={removing} host={hosts.byId.get(removing.hostId)?.name ?? ""} onClose={() => setRemoving(null)} onRemoved={() => { if (removing.id === openId) setParam("image", null); }} />}
    </>
  );
}
