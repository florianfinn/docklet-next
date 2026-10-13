import { Archive, ArrowLeft, Box, Database, FolderOpen, Info, Layers, LayoutGrid, Trash2, Unplug } from "lucide-react";
import { useState } from "react";
import { Link, useNavigate, useParams, useSearchParams } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";

import { useContainers, useSettings, useVolume } from "@/data/queries";
import { formatBytes, formatDate } from "@/domain/format";
import { formatPercentNumber } from "@/domain/locale";
import type { Host } from "@/domain/model";
import { holdsDatabase, volumeLabel, type VolumeDetail } from "@/domain/volumes";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { HostChip } from "@/shared/components/host-chip";
import { Meter } from "@/shared/components/meter";
import { EmptyState, QueryError } from "@/shared/components/page";
import { IconTile } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/shared/ui/table";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { BackupHistory } from "./backups/BackupHistory";
import { BackupPolicyCard } from "./backups/BackupPolicyCard";
import { FileWorkspace } from "./files/FileWorkspace";
import { BackupBadge, KindBadge } from "./storage-visuals";
import { useVolumeCommands } from "./use-volume-commands";
import { deleteBlock } from "./volume-menu";

type Tab = "overview" | "files" | "backups";

function OverviewTab({ volume: v, host }: { volume: VolumeDetail; host: Host | undefined }) {
  const t = useTranslations("volumes.detail");
  const cmd = useVolumeCommands();
  const navigate = useNavigate();
  const [confirm, setConfirm] = useState(false);
  const share = host ? (v.sizeBytes / (host.diskTotalGb * 1024 ** 3)) * 100 : 0;
  const block = deleteBlock(v, host);
  const blocked = block === "bind" ? t("deleteBind") : block === "in-use" ? t("deleteBlocked") : block === "offline" ? t("deleteOffline") : undefined;
  const rows: [string, string][] = [
    [t("driver"), v.driver ?? t("none")],
    [v.kind === "volume" ? t("mountpoint") : t("source"), v.source],
    [t("compose"), v.composeName ? `${v.stackId}/${v.composeName}` : t("none")],
    [t("created"), formatDate(v.createdAt)],
  ];
  return (
    <div className="grid gap-4 lg:grid-cols-3">
      <Card className="min-w-0 gap-3 py-4 lg:col-span-2">
        <CardHeader className="px-4">
          <CardTitle>{t("consumers")}</CardTitle>
        </CardHeader>
        <CardContent className="px-2">
          {v.consumers.length === 0 ? (
            <p className="px-2 text-sm text-muted-foreground">{t("consumersNone")}</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>{t("container")}</TableHead>
                  <TableHead>{t("target")}</TableHead>
                  <TableHead className="text-right">
                    <span className="sr-only">{t("links")}</span>
                  </TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {v.consumers.map((c) => (
                  <TableRow key={`${c.containerId}-${c.target}`}>
                    <TableCell>
                      <span className="flex items-center gap-2 font-medium">
                        {c.stackId ? <Layers className="size-3.5 text-muted-foreground" aria-hidden /> : <Box className="size-3.5 text-muted-foreground" aria-hidden />}
                        {c.containerName}
                      </span>
                      {!c.running && <span className="text-[11px] text-muted-foreground">{t("stopped")}</span>}
                    </TableCell>
                    <TableCell className="font-mono text-xs">
                      {c.target}
                      {c.readOnly && <span className="ml-1.5 rounded border px-1 text-[10px] text-muted-foreground">ro</span>}
                    </TableCell>
                    <TableCell className="text-right">
                      <div className="flex justify-end gap-1">
                        {c.stackId && (
                          <Button asChild size="sm" variant="ghost" className="h-7 text-xs">
                            <Link to={`/stacks/${c.stackId}`}>{t("openStack")}</Link>
                          </Button>
                        )}
                        <Button asChild size="sm" variant="ghost" className="h-7 text-xs">
                          <Link to={`/containers?c=${c.containerId}`}>{t("openContainer")}</Link>
                        </Button>
                      </div>
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}
        </CardContent>
      </Card>
      <Card className="min-w-0 gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle>{t("usage")}</CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-3 px-4">
          <div className="text-2xl font-semibold tracking-tight tabular">{formatBytes(v.sizeBytes)}</div>
          <Meter label={t("usage")} value={Math.max(share, 0.5)} color={host?.color} sub={t("usageOfHost", { percent: `${share < 0.1 ? "< " : ""}${formatPercentNumber(Math.max(share, 0.1), { maximumFractionDigits: 1 })}`, total: `${host?.diskTotalGb ?? "–"} GB` })} />
          <p className="text-[11px] text-muted-foreground">{t("usageSub")}</p>
        </CardContent>
      </Card>
      <Card className="min-w-0 gap-3 py-4 lg:col-span-3">
        <CardHeader className="px-4">
          <CardTitle className="flex items-center gap-2">
            <Info className="size-4 text-muted-foreground" aria-hidden />
            {t("properties")}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-4 px-4">
          <dl className="grid gap-x-6 gap-y-2 text-sm sm:grid-cols-[auto_1fr]">
            {rows.map(([k, val]) => (
              <div key={k} className="contents">
                <dt className="text-muted-foreground">{k}</dt>
                <dd className="font-mono text-xs break-all">{val}</dd>
              </div>
            ))}
            <dt className="text-muted-foreground">{t("labels")}</dt>
            <dd className="flex flex-wrap gap-1 font-mono text-[11.5px]">
              {Object.entries(v.labels).length
                ? Object.entries(v.labels).map(([k, val]) => (
                    <span key={k} className="max-w-full break-all rounded-md border bg-muted/30 px-1.5 py-0.5">
                      {k}={val}
                    </span>
                  ))
                : t("none")}
            </dd>
          </dl>
          {v.kind === "bind" && <p className="text-xs text-muted-foreground">{v.fileBind ? t("fileBindNote") : t("bindNote")}</p>}
          <div className="flex border-t pt-3">
            <Tooltip>
              <TooltipTrigger asChild>
                <span>
                  <Button variant="ghost" className="text-destructive hover:text-destructive" disabled={!!blocked} onClick={() => setConfirm(true)}>
                    <Trash2 data-icon="inline-start" />
                    {t("delete")}
                  </Button>
                </span>
              </TooltipTrigger>
              {blocked && <TooltipContent>{blocked}</TooltipContent>}
            </Tooltip>
          </div>
        </CardContent>
      </Card>
      <ConfirmDialog
        open={confirm}
        onOpenChange={setConfirm}
        title={t("deleteTitle", { name: volumeLabel(v) })}
        description={t("deleteBody")}
        confirmLabel={t("deleteConfirm")}
        destructive
        onConfirm={async () => {
          if (!(await cmd.remove(v.id))) return false;
          toast.success(t("deleted", { name: volumeLabel(v) }));
          navigate("/volumes");
        }}
      />
    </div>
  );
}

export default function VolumePage() {
  const { volumeId } = useParams();
  const t = useTranslations("volumes.detail");
  const tf = useTranslations("volumes.files");
  const volume = useVolume(volumeId);
  const settings = useSettings();
  const containers = useContainers();
  const hosts = useHostScope();
  const [params, setParams] = useSearchParams();
  const tab = (["overview", "files", "backups"].includes(params.get("tab") ?? "") ? params.get("tab") : "overview") as Tab;
  const v = volume.data;
  const host = v ? hosts.byId.get(v.hostId) : undefined;
  const online = host?.status === "online";
  const images = (containers.data ?? []).filter((c) => v?.consumers.some((x) => x.containerId === c.id)).map((c) => c.image.repository);
  const runningNames = v?.consumers.filter((c) => c.running).map((c) => c.containerName) ?? [];
  const warning = v && holdsDatabase(images) ? t("dbWarning", { images: [...new Set(images)].join(", ") }) : runningNames.length ? tf("runningWarning", { names: runningNames.join(", ") }) : undefined;

  return (
    <>
      <Button asChild variant="ghost" size="sm" className="group mb-3 -ml-2 text-muted-foreground">
        <Link to="/volumes">
          <ArrowLeft data-icon="inline-start" className="transition-transform duration-(--dur) group-hover:-translate-x-0.5" />
          {t("back")}
        </Link>
      </Button>
      {volume.isPending ? (
        <div className="flex flex-col gap-4">
          <Skeleton className="h-16 w-96 max-w-full rounded-xl" />
          <Skeleton className="h-9 w-80 rounded-lg" />
          <Skeleton className="h-80 rounded-2xl" />
        </div>
      ) : volume.isError ? (
        volume.error.message === "not-found" ? (
          <EmptyState icon={<Database />} title={t("notFound")} description={t("notFoundBody")} />
        ) : (
          <QueryError onRetry={() => void volume.refetch()} />
        )
      ) : (
        v && (
          <>
            <header data-page-actions className="mb-5 flex flex-wrap items-start gap-4">
              <IconTile color={host?.color} size="lg">
                {v.kind === "volume" ? <Database /> : <FolderOpen />}
              </IconTile>
              <div className="min-w-0 flex-1">
                <div className="flex flex-wrap items-center gap-2">
                  <h1 className="truncate text-2xl font-semibold tracking-tight">{volumeLabel(v)}</h1>
                  <KindBadge volume={v} />
                  <BackupBadge volume={v} />
                </div>
                <div className="mt-1.5 flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                  <HostChip host={host} />
                  <span className="font-mono">{v.kind === "volume" ? v.name : v.source}</span>
                  <span>{formatBytes(v.sizeBytes)}</span>
                </div>
              </div>
            </header>
            {!online && (
              <Alert className="mb-4 border-orange/30 bg-orange/8">
                <Unplug className="text-orange" />
                <AlertDescription>{t("offline", { host: host?.name ?? v.hostId })}</AlertDescription>
              </Alert>
            )}
            <Tabs value={tab} onValueChange={(next) => setParams({ tab: next }, { replace: true })}>
              <TabsList aria-label={t("tabsLabel")} className="mb-4 max-w-full flex-wrap justify-start group-data-[orientation=horizontal]/tabs:h-auto">
                <TabsTrigger value="overview" className="gap-1.5 px-3">
                  <LayoutGrid /> {t("tabs.overview")}
                </TabsTrigger>
                <TabsTrigger value="files" className="gap-1.5 px-3">
                  <FolderOpen /> {t("tabs.files")}
                </TabsTrigger>
                <TabsTrigger value="backups" className="gap-1.5 px-3">
                  <Archive /> {t("tabs.backups")}
                </TabsTrigger>
              </TabsList>
              <TabsContent value="overview">
                <OverviewTab volume={v} host={host} />
              </TabsContent>
              <TabsContent value="files">
                {online ? <FileWorkspace volume={v} warning={warning} /> : <EmptyState icon={<Unplug />} title={tf("offline")} />}
              </TabsContent>
              <TabsContent value="backups" className="flex flex-col gap-4">
                {settings.data ? (
                  <>
                    <BackupPolicyCard key={JSON.stringify(v.backup ?? null)} volume={v} targets={settings.data.backups.targets} defaultRetention={settings.data.backups.defaultRetention} hostOnline={online} />
                    <BackupHistory volume={v} targets={settings.data.backups.targets} hostOnline={online} />
                  </>
                ) : (
                  <Skeleton className="h-72 rounded-2xl" />
                )}
              </TabsContent>
            </Tabs>
          </>
        )
      )}
    </>
  );
}
