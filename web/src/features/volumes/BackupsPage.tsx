import { Archive, CircleAlert, HardDrive, Play, ServerCog, ShieldCheck, ShieldOff } from "lucide-react";
import { useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useSettings, useVolumes } from "@/data/queries";
import { filterByHosts } from "@/domain/hosts";
import { volumeLabel } from "@/domain/volumes";
import { StatStrip } from "@/shared/component-set/stat-strip";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, OverviewTable, type OverviewColumn, type TableView } from "@/shared/components/overview/overview-table";
import { EmptyState, PageHeader, QueryError } from "@/shared/components/page";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { useRowNavigation } from "@/shared/lib/row-navigation";
import { nextSort } from "@/shared/lib/table-sort";
import { useHostSelection } from "@/shared/preferences/preferences-store";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { Skeleton } from "@/shared/ui/skeleton";

import { backupSortDefaultDir, DEFAULT_BACKUP_SORT, sortBackups, type BackupSort, type BackupSortKey } from "./backup-sort";
import { BackupBadge, KindBadge } from "./storage-visuals";
import { useVolumeCommands } from "./use-volume-commands";

/** All backup plans of the filtered hosts plus the configured targets. */
export default function BackupsPage() {
  const t = useTranslations("volumes.backups");
  const dt = useAdapterText();
  const volumes = useVolumes();
  const settings = useSettings();
  const hosts = useHostScope();
  const selection = useHostSelection();
  const cmd = useVolumeCommands();
  const text = useScheduleText();
  const rowNav = useRowNavigation();
  const timeZone = useHubTimeZone();
  const [now] = useState(() => new Date());
  const [sort, setSort] = useState<BackupSort>(DEFAULT_BACKUP_SORT);
  const onSort = (key: BackupSortKey) => setSort((s) => nextSort(s, key, backupSortDefaultDir));
  const visible = useMemo(() => filterByHosts(volumes.data ?? [], selection), [volumes.data, selection]);
  const targets = useMemo(() => settings.data?.backups.targets ?? [], [settings.data]);
  const planned = useMemo(() => sortBackups(visible.filter((v) => v.backup), hosts.byId, targets, sort, now, timeZone), [visible, hosts.byId, targets, sort, now, timeZone]);
  const unprotected = visible.filter((v) => !v.backup && v.consumers.length > 0);
  const failed = planned.filter((v) => v.lastBackup?.status === "failed").length;
  const key = (k: BackupSortKey) => ({ key: k, label: t(`columns.${k === "name" ? "volume" : k}`) });
  const columns = (view: TableView): OverviewColumn<BackupSortKey>[] =>
    view === "condensed"
      ? [
          { id: "name", label: t("columns.volume"), sorts: [key("name"), key("host")], className: "min-w-44" },
          { id: "target", label: t("columns.target"), sorts: [key("target")], wide: true },
          { id: "plan", label: t("columns.schedule"), sorts: [key("schedule"), key("next")], wide: true },
          { id: "last", label: t("columns.last"), sorts: [key("last")] },
        ]
      : [
          { id: "name", label: t("columns.volume"), sorts: [key("name")], className: "min-w-44" },
          { id: "host", label: t("columns.host"), sorts: [key("host")] },
          { id: "target", label: t("columns.target"), sorts: [key("target")], wide: true },
          { id: "schedule", label: t("columns.schedule"), sorts: [key("schedule")] },
          { id: "next", label: t("columns.next"), sorts: [key("next")] },
          { id: "last", label: t("columns.last"), sorts: [key("last")] },
        ];

  return (
    <>
      <PageHeader
        title={t("title")}
        description={t("description")}
        actions={
          <Button asChild variant="outline">
            <Link to="/settings/backups">
              <HardDrive data-icon="inline-start" />
              {t("manageTargets")}
            </Link>
          </Button>
        }
      />
      <StatStrip
        items={[
          { label: t("policy"), value: volumes.isPending ? "–" : planned.length, icon: <Archive /> },
          { label: t("status.succeeded"), value: volumes.isPending ? "–" : planned.filter((v) => v.lastBackup?.status === "succeeded").length, icon: <ShieldCheck />, tone: "ok" },
          { label: t("status.failed"), value: volumes.isPending ? "–" : failed, icon: <CircleAlert />, tone: failed ? "err" : undefined },
          { label: t("unprotected", { count: unprotected.length }), value: volumes.isPending ? "–" : unprotected.length, icon: <ShieldOff />, tone: unprotected.length ? "warn" : undefined },
        ]}
      />
      <div className="grid items-start gap-4 xl:grid-cols-[minmax(0,1fr)_22rem]">
        <Card className="min-w-0 gap-3 p-4">
          {volumes.isPending ? (
            <Skeleton className="h-72 rounded-xl" />
          ) : volumes.isError ? (
            <QueryError onRetry={() => void volumes.refetch()} />
          ) : planned.length === 0 ? (
            <EmptyState icon={<Archive />} title={t("overviewEmpty")} />
          ) : (
            <OverviewTable fitKey={`backups|${planned.length}`} columns={columns} sort={sort} onSort={onSort} sortLabel={(column) => t("sortBy", { column })} selecting={false} actionsLabel={t("columns.actions")} dense={false} listLabel={t("title")}>
              {(view, cols) =>
                planned.map((v) => {
                  const plan = v.backup!;
                  const host = hosts.byId.get(v.hostId);
                  const next = plan.enabled ? text.next(plan.schedule, now, timeZone) : undefined;
                  const nextCell = next ? (
                    <span className="flex flex-col text-xs" title={next.zone}>
                      <span>{next.relative}</span>
                      <span className="text-[11px] text-muted-foreground">{next.when}</span>
                    </span>
                  ) : (
                    <span className="text-xs text-muted-foreground">{plan.enabled ? t("nextManual") : t("disabled")}</span>
                  );
                  const schedule = <span className="text-xs">{text.summary(plan.schedule)}</span>;
                  return (
                    <OverviewRowView
                      key={v.id}
                      view={view}
                      columns={cols}
                      pad="py-2.5"
                      hostColor={host?.color}
                      onClick={rowNav(`/volumes/${v.id}?tab=backups`).onClick}
                      cells={{
                        name: (
                          <div className="min-w-0">
                            <Link to={`/volumes/${v.id}?tab=backups`} className="font-medium [overflow-wrap:anywhere] hover:underline">
                              {volumeLabel(v)}
                            </Link>
                            <div className="mt-0.5 flex flex-wrap items-center gap-2">
                              <KindBadge volume={v} className="h-5 px-1.5 text-[10.5px]" />
                              {view === "condensed" && <HostChip host={host} />}
                            </div>
                          </div>
                        ),
                        host: <HostChip host={host} />,
                        target: <span className="text-xs [overflow-wrap:anywhere]">{targets.find((x) => x.id === plan.targetId)?.name ?? "–"}</span>,
                        schedule,
                        next: nextCell,
                        plan: (
                          <span className="flex flex-col gap-0.5">
                            {schedule}
                            {nextCell}
                          </span>
                        ),
                        last: <BackupBadge volume={v} />,
                      }}
                      actions={
                        <Button size="sm" variant="outline" disabled={host?.status !== "online"} onClick={() => void cmd.backup(v.id)}>
                          <Play data-icon="inline-start" />
                          {t("runNow")}
                        </Button>
                      }
                    />
                  );
                })
              }
            </OverviewTable>
          )}
          {unprotected.length > 0 && (
            <p className="flex flex-wrap items-center gap-2 px-1 text-xs text-muted-foreground">
              <ShieldOff className="size-3.5 text-warning" aria-hidden />
              {t("unprotected", { count: unprotected.length })}: {unprotected.slice(0, 6).map(volumeLabel).join(", ")}
              {unprotected.length > 6 && " …"}
            </p>
          )}
        </Card>
        <Card className="min-w-0 gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2">
              <ServerCog className="size-4 text-muted-foreground" aria-hidden />
              {t("targets")}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-2 px-4">
            {settings.isPending ? (
              <Skeleton className="h-40 rounded-xl" />
            ) : (
              targets.map((tg) => (
                <div key={tg.id} className="rounded-xl border bg-card/40 p-3">
                  <div className="flex items-center gap-2">
                    <span className="min-w-0 flex-1 truncate text-sm font-medium">{tg.name}</span>
                    <ToneBadge tone={tg.lastTest ? (tg.lastTest.ok ? "ok" : "err") : "gray"} className="h-5 px-1.5 text-[10.5px]">
                      {t(`targetKinds.${tg.kind}`)}
                    </ToneBadge>
                  </div>
                  <div className="mt-1 truncate font-mono text-[11.5px] text-muted-foreground">
                    {tg.kind === "smb" ? `//${tg.server}/${tg.share}${tg.path}` : tg.kind === "nfs" ? `${tg.server}:${tg.path}` : tg.path}
                  </div>
                  <div className="mt-1 text-[11px] text-muted-foreground">{tg.lastTest ? `${tg.lastTest.ok ? t("targetTestOk") : t("targetTestFailed")} · ${dt(tg.lastTest.message)}` : t("targetUntested")}</div>
                </div>
              ))
            )}
          </CardContent>
        </Card>
      </div>
    </>
  );
}
