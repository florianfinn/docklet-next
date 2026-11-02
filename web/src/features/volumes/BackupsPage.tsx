// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original independent host overview retaining sortable snapshot rows; M16 figures, unprotected entries, per-row backup and targets panel after the approved prototype.

import { Archive, CircleAlert, HardDrive, Play, ServerCog, ShieldCheck, ShieldOff } from "lucide-react";
import { useState } from "react";
import { Link } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import { useBackupOverviews, useBackupTargets } from "@/data/backup-queries";
import { useVolumes } from "@/data/queries";
import { isHostAvailable } from "@/domain/hosts";
import { volumeLabel } from "@/domain/volumes";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { ToneBadge } from "@/shared/components/tone";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, OverviewTable } from "@/shared/components/overview/overview-table";
import { PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { nextSort } from "@/shared/lib/table-sort";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { backupSortDefaultDir, DEFAULT_BACKUP_SORT, sortBackups, type BackupSort, type BackupSortKey } from "./backup-sort";
import { backupFigures, newestCheck, targetAddress } from "./backup-figures";
import { BackupBadge } from "./storage-visuals";
import { useVolumeCommands } from "./use-volume-commands";

export default function BackupsPage() {
  return <CapabilityGate operation="listBackupOverviews"><BackupOverview /></CapabilityGate>;
}
function BackupOverview() {
  const locale = useLocale();
  const t = useTranslations("volumes.backups"), hosts = useHostScope(), overviews = useBackupOverviews(), text = useScheduleText();
  const inventory = useVolumes(), targets = useBackupTargets(), cmd = useVolumeCommands();
  const [sort, setSort] = useState<BackupSort>(DEFAULT_BACKUP_SORT);
  const visible = overviews.data?.filter((row) => hosts.visible.some((host) => host.id === row.hostId)) ?? [];
  const figures = backupFigures(visible, inventory.data ?? []);
  const volumes = visible.flatMap((row) => row.volumes).filter((v) => v.backup || v.lastBackup || v.volumeExists === false);
  const rows = sortBackups(volumes, hosts.byId, visible.flatMap((row) => row.targets), sort);
  const key = (k: BackupSortKey) => ({ key: k, label: t(`columns.${k === "name" ? "volume" : k}`) });
  return <>
    <PageHeader
      title={t("title")}
      description={t("description")}
      actions={<Button asChild variant="outline"><Link to="/settings/backups"><HardDrive data-icon="inline-start" />{t("manageTargets")}</Link></Button>}
    />
    <StatStrip
      items={[
        { label: t("policy"), value: overviews.isPending ? "–" : figures.plans, icon: <Archive /> },
        { label: t("status.succeeded"), value: overviews.isPending ? "–" : figures.succeeded, icon: <ShieldCheck />, tone: "ok" },
        { label: t("status.failed"), value: overviews.isPending ? "–" : figures.failed, icon: <CircleAlert />, tone: figures.failed ? "err" : undefined },
        { label: t("unprotected", { count: figures.unprotected.length }), value: overviews.isPending || inventory.isPending ? "–" : figures.unprotected.length, icon: <ShieldOff />, tone: figures.unprotected.length ? "warn" : undefined },
      ]}
    />
    <div className="mb-4 space-y-2">
      {hosts.visible.map((host) => {
        const observed = visible.find((row) => row.hostId === host.id);
        return !observed || observed.unavailable
          ? <p key={host.id} role="status" className="rounded-xl border p-3 text-sm">
              {host.name}: {observed ? t("hostUnavailable") : t("hostLoading")}
            </p>
          : null;
      })}
    </div>
    <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
    {overviews.isError
      ? <QueryError error={overviews.error} onRetry={() => void overviews.refetch()} />
      : <Card className="min-w-0 py-4">
          <CardContent className="flex flex-col gap-3 px-3">
            {!rows.length
              ? !overviews.isPending && !visible.some((row) => row.unavailable) && <p>{t("overviewEmpty")}</p>
              : <OverviewTable
                  fitKey={`backups|${rows.length}`}
                  columns={() => [
                    { id: "name", label: t("columns.volume"), sorts: [key("name")] },
                    { id: "host", label: t("columns.host"), sorts: [key("host")] },
                    { id: "target", label: t("columns.target"), sorts: [key("target")] },
                    { id: "schedule", label: t("columns.schedule"), sorts: [key("schedule")] },
                    { id: "next", label: t("columns.next"), sorts: [key("next")] },
                    { id: "last", label: t("columns.last"), sorts: [key("last")] },
                  ]}
                  sort={sort}
                  onSort={(key) => setSort((old) => nextSort(old, key, backupSortDefaultDir))}
                  sortLabel={(column) => t("sortBy", { column })}
                  dense={false}
                  selecting={false}
                  actionsLabel={t("columns.actions")}
                  listLabel={t("title")}
                >
                  {(view, columns) => rows.map((volume) => {
                    const overview = visible.find((row) => row.hostId === volume.hostId)!;
                    const plan = volume.backup;
                    const online = isHostAvailable(hosts.byId.get(volume.hostId));
                    return <OverviewRowView
                      key={volume.id}
                      view={view}
                      columns={columns}
                      pad="py-3"
                      actions={plan && <Button size="sm" variant="outline" disabled={!online || !!volume.running || volume.volumeExists === false} onClick={() => void cmd.backup(volume.id)}>
                        <Play data-icon="inline-start" />
                        {t("runNow")}
                      </Button>}
                      cells={{
                        name: <div>
                          <Link
                            className="break-all font-medium hover:underline"
                            to={`/volumes/${encodeURIComponent(volume.id)}?tab=backups`}
                          >
                            {volumeLabel(volume)}
                          </Link>
                          {volume.volumeExists === false && <p className="text-xs">{t("volumeMissing")}</p>}
                        </div>,
                        host: <HostChip host={hosts.byId.get(volume.hostId)} />,
                        target: <span className="text-xs">
                          {overview.targets.find((target) => target.id === plan?.targetId)?.name ?? "–"}
                        </span>,
                        schedule: <span className="text-xs">
                          {!plan?.enabled
                            ? t("nextManual")
                            : plan.schedule.mode === "custom"
                              ? t("customSchedule")
                              : text.summary(plan.schedule)}
                        </span>,
                        next: <span className="text-xs">
                          {plan?.nextRunAt
                            ? `${new Intl.DateTimeFormat(locale, {
                                dateStyle: "medium",
                                timeStyle: "short",
                                timeZone: overview.timeZone,
                              }).format(new Date(plan.nextRunAt))} · ${overview.timeZone}`
                            : t("nextManual")}
                        </span>,
                        last: <BackupBadge volume={volume} />,
                      }}
                    />;
                  })}
                </OverviewTable>}
            {figures.unprotected.length > 0 && <p className="flex flex-wrap items-center gap-2 px-1 text-xs text-muted-foreground">
              <ShieldOff className="size-3.5 text-warning" aria-hidden />
              {t("unprotected", { count: figures.unprotected.length })}: {figures.unprotected.slice(0, 6).map(volumeLabel).join(", ")}
              {figures.unprotected.length > 6 && " …"}
            </p>}
          </CardContent>
        </Card>}
      <Card className="gap-3 py-4">
        <CardHeader className="px-4">
          <CardTitle className="flex items-center gap-2">
            <ServerCog className="size-4 text-muted-foreground" aria-hidden />
            {t("targets")}
          </CardTitle>
        </CardHeader>
        <CardContent className="flex flex-col gap-2 px-4">
          {targets.isError
            ? <QueryError error={targets.error} onRetry={() => void targets.refetch()} />
            : (targets.data ?? []).filter((target) => !target.retiredAt).map((target) => {
                const check = newestCheck(target);
                return <div key={target.id} className="rounded-xl border bg-card/40 p-3">
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{target.name}</span>
                    <ToneBadge tone={check ? (check.ok ? "ok" : "err") : "gray"} className="h-5 px-1.5 text-[10.5px]">{t(`targetKinds.${target.kind}`)}</ToneBadge>
                  </div>
                  <div className="mt-1 truncate font-mono text-[11.5px] text-muted-foreground">{targetAddress(target, hosts.byId.get(target.hostId ?? "")?.name)}</div>
                  <div className="mt-1 text-[11px] text-muted-foreground">
                    {check
                      ? `${check.ok ? t("targetTestOk") : t("targetTestFailed")} · ${new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short" }).format(new Date(check.at))}`
                      : t("targetUntested")}
                  </div>
                </div>;
              })}
        </CardContent>
      </Card>
    </div>
  </>;
}
