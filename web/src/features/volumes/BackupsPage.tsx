// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original independent host overview retaining sortable snapshot rows.

import { useState } from "react";
import { Link } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import { useBackupOverviews } from "@/data/backup-queries";
import { volumeLabel } from "@/domain/volumes";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { HostChip } from "@/shared/components/host-chip";
import { OverviewRowView, OverviewTable } from "@/shared/components/overview/overview-table";
import { PageHeader, QueryError } from "@/shared/components/page";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { nextSort } from "@/shared/lib/table-sort";
import { Button } from "@/shared/ui/button";
import { Card, CardContent } from "@/shared/ui/card";
import { backupSortDefaultDir, DEFAULT_BACKUP_SORT, sortBackups, type BackupSort, type BackupSortKey } from "./backup-sort";
import { BackupBadge } from "./storage-visuals";

export default function BackupsPage() {
  return <CapabilityGate operation="listBackupOverviews"><BackupOverview /></CapabilityGate>;
}
function BackupOverview() {
  const locale = useLocale();
  const t = useTranslations("volumes.backups"), hosts = useHostScope(), overviews = useBackupOverviews(), text = useScheduleText();
  const [sort, setSort] = useState<BackupSort>(DEFAULT_BACKUP_SORT);
  const visible = overviews.data?.filter((row) => hosts.visible.some((host) => host.id === row.hostId)) ?? [];
  const volumes = visible.flatMap((row) => row.volumes).filter((v) => v.backup || v.lastBackup || v.volumeExists === false);
  const rows = sortBackups(volumes, hosts.byId, visible.flatMap((row) => row.targets), sort);
  const key = (k: BackupSortKey) => ({ key: k, label: t(`columns.${k === "name" ? "volume" : k}`) });
  return <><PageHeader title={t("title")} description={t("description")} actions={<Button asChild variant="outline"><Link to="/settings/backups">{t("manageTargets")}</Link></Button>} />
    <div className="mb-4 space-y-2">{hosts.visible.map((host) => { const observed = visible.find((row) => row.hostId === host.id); return !observed || observed.unavailable ? <p key={host.id} role="status" className="rounded-xl border p-3 text-sm">{host.name}: {observed ? t("hostUnavailable") : t("hostLoading")}</p> : null; })}</div>
    {overviews.isError ? <QueryError error={overviews.error} onRetry={() => void overviews.refetch()} /> : <Card className="py-4"><CardContent className="px-3">
      {!rows.length ? !overviews.isPending && !visible.some((row) => row.unavailable) && <p>{t("overviewEmpty")}</p> : <OverviewTable fitKey={`backups|${rows.length}`} columns={() => [
        { id: "name", label: t("columns.volume"), sorts: [key("name")] }, { id: "host", label: t("columns.host"), sorts: [key("host")] }, { id: "target", label: t("columns.target"), sorts: [key("target")] }, { id: "schedule", label: t("columns.schedule"), sorts: [key("schedule")] }, { id: "next", label: t("columns.next"), sorts: [key("next")] }, { id: "last", label: t("columns.last"), sorts: [key("last")] },
      ]} sort={sort} onSort={(key) => setSort((old) => nextSort(old, key, backupSortDefaultDir))} sortLabel={(column) => t("sortBy", { column })} dense={false} selecting={false} actionsLabel={t("columns.actions")} listLabel={t("title")}>
        {(view, columns) => rows.map((volume) => { const overview = visible.find((row) => row.hostId === volume.hostId)!; const plan = volume.backup; return <OverviewRowView key={volume.id} view={view} columns={columns} pad="py-3" actions={null} cells={{
          name: <div><Link className="break-all font-medium hover:underline" to={`/volumes/${encodeURIComponent(volume.id)}?tab=backups`}>{volumeLabel(volume)}</Link>{volume.volumeExists === false && <p className="text-xs">{t("volumeMissing")}</p>}</div>,
          host: <HostChip host={hosts.byId.get(volume.hostId)} />,
          target: <span className="text-xs">{overview.targets.find((target) => target.id === plan?.targetId)?.name ?? "–"}</span>,
          schedule: <span className="text-xs">{plan?.schedule.mode === "custom" ? t("customSchedule") : plan ? text.summary(plan.schedule) : t("nextManual")}</span>,
          next: <span className="text-xs">{plan?.nextRunAt ? `${new Intl.DateTimeFormat(locale, { dateStyle: "medium", timeStyle: "short", timeZone: overview.timeZone }).format(new Date(plan.nextRunAt))} · ${overview.timeZone}` : t("nextManual")}</span>,
          last: <BackupBadge volume={volume} />,
        }} />; })}
      </OverviewTable>}
    </CardContent></Card>}
  </>;
}
