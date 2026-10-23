import { CapabilityGate } from "@/shared/components/capability-gate";
import { History, Loader2, RotateCcw } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { formatBytes, formatDateTime, formatRelative } from "@/domain/format";
import type { BackupTarget } from "@/domain/settings";
import type { BackupRun, VolumeDetail } from "@/domain/volumes";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ChoiceCards } from "@/shared/components/form-field";
import { ToneBadge } from "@/shared/components/tone";
import { useAdapterText } from "@/data/use-adapter-text";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { OverviewRowView, OverviewTable } from "@/shared/components/overview/overview-table";

import { useVolumeCommands } from "../use-volume-commands";

const STATUS_TONE = { running: "accent", succeeded: "ok", failed: "err" } as const;

export function BackupHistory(props: { volume: VolumeDetail; targets: BackupTarget[]; hostOnline: boolean }) {
  return <CapabilityGate operation="restoreBackup"><ConnectedBackupHistory {...props} /></CapabilityGate>;
}

function ConnectedBackupHistory({ volume, targets, hostOnline }: { volume: VolumeDetail; targets: BackupTarget[]; hostOnline: boolean }) {
  const t = useTranslations("volumes.backups");
  const dt = useAdapterText();
  const cmd = useVolumeCommands();
  const [restore, setRestore] = useState<BackupRun | null>(null);
  const [mode, setMode] = useState<"replace" | "new">("replace");
  const running = volume.consumers.filter((c) => c.running).map((c) => c.containerName);
  const targetName = (id: string) => targets.find((x) => x.id === id)?.name ?? id;

  return (
    <Card className="gap-3 py-4">
      <CardHeader className="px-4">
        <CardTitle className="flex items-center gap-2">
          <History className="size-4 text-muted-foreground" aria-hidden />
          {t("history")}
        </CardTitle>
      </CardHeader>
      <CardContent className="px-2">
        {volume.backups.length === 0 ? (
          <p className="px-2 py-4 text-sm text-muted-foreground">{t("historyEmpty")}</p>
        ) : (
          <OverviewTable
            fitKey={`history|${volume.backups.length}`}
            columns={() => [
              { id: "name", label: t("columns.at") },
              { id: "status", label: t("columns.status") },
              { id: "trigger", label: t("columns.trigger") },
              { id: "size", label: t("columns.size") },
              { id: "archive", label: t("columns.archive"), className: "max-w-60", wide: true },
            ]}
            selecting={false}
            actionsLabel={t("columns.actions")}
            dense
            listLabel={t("history")}
          >
            {(view, cols) =>
              volume.backups.map((r) => (
                <OverviewRowView
                  key={r.id}
                  view={view}
                  columns={cols}
                  pad="py-2"
                  className="cursor-default"
                  cells={{
                    name: (
                      <span className="text-xs">
                        <span title={formatDateTime(r.startedAt)}>{formatRelative(r.startedAt)}</span>
                        <span className="block text-[11px] text-muted-foreground">{targetName(r.targetId)}</span>
                      </span>
                    ),
                    status: (
                      <>
                        <ToneBadge tone={STATUS_TONE[r.status]}>
                          {r.status === "running" && <Loader2 className="animate-spin" aria-hidden />}
                          {t(`status.${r.status}`)}
                        </ToneBadge>
                        {r.error && <span className="mt-1 block max-w-64 text-[11px] text-destructive">{dt(r.error)}</span>}
                      </>
                    ),
                    trigger: (
                      <span className="text-xs">
                        {t(`trigger.${r.trigger}`)}
                        {r.stoppedContainers && <span className="block text-[11px] text-muted-foreground">{t("stopped")}</span>}
                      </span>
                    ),
                    size: <span className="font-mono text-xs tabular">{r.sizeBytes !== undefined ? formatBytes(r.sizeBytes) : "–"}</span>,
                    archive: <span className="font-mono text-[11.5px] text-muted-foreground [overflow-wrap:anywhere]">{r.archive ?? "–"}</span>,
                  }}
                  actions={
                    r.status === "succeeded" ? (
                      <Button
                        size="sm"
                        variant="outline"
                        disabled={!hostOnline}
                        onClick={() => {
                          setMode("replace");
                          setRestore(r);
                        }}
                      >
                        <RotateCcw data-icon="inline-start" />
                        {t("restore")}
                      </Button>
                    ) : null
                  }
                />
              ))
            }
          </OverviewTable>
        )}
      </CardContent>
      <ConfirmDialog
        open={!!restore}
        onOpenChange={(o) => !o && setRestore(null)}
        title={t("restoreTitle", { archive: restore?.archive ?? "" })}
        description={t("restoreBody")}
        confirmLabel={t("restoreConfirm")}
        onConfirm={async () => !!restore && !!(await cmd.restore(volume.id, restore.id, mode))}
      >
        <ChoiceCards
          label={t("restore")}
          value={mode}
          onChange={setMode}
          columns={2}
          options={[
            { value: "replace", title: t("restoreReplace"), description: running.length ? t("restoreReplaceHint", { names: running.join(", ") }) : t("restoreReplaceHintIdle") },
            ...(volume.kind === "volume" ? [{ value: "new" as const, title: t("restoreNew"), description: t("restoreNewHint") }] : []),
          ]}
        />
        {volume.kind === "bind" && <p className="text-xs text-muted-foreground">{t("restoreNewBind")}</p>}
        <p className="text-[11px] text-muted-foreground">{t("restoreDemo")}</p>
      </ConfirmDialog>
    </Card>
  );
}
