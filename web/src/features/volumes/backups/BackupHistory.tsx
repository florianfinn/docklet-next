// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live snapshot actions and outcome presentation.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { backupCodeKey, backupErrorMessage } from "@/data/backup-error";
import { formatBytes, formatDateTime } from "@/domain/format";
import type { BackupRun, UsableBackupTarget, VolumeDetail } from "@/domain/volumes";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { ToneBadge } from "@/shared/components/tone";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { RestoreDialog } from "./RestoreDialog";
import { RestoreFilesSheet } from "./RestoreFilesSheet";
import { useVolumeCapability } from "../use-volume-capability";

const STATUS_TONE = { running: "accent", succeeded: "ok", failed: "err", partial: "warn", unavailable: "gray" } as const;

export function BackupHistory({ volume, targets, hostOnline, accepted = () => {} }: { volume: VolumeDetail; targets: UsableBackupTarget[]; hostOnline: boolean; accepted?: (id: string) => void }) {
  const t = useTranslations("volumes.backups"), all = useTranslations(), adapter = useAdapter();
  const restoreCapability = useVolumeCapability("restoreBackup", volume.id), filesCapability = useVolumeCapability("restoreBackupFiles", volume.id), downloadCapability = useVolumeCapability("downloadBackup", volume.id), deleteCapability = useVolumeCapability("deleteBackup", volume.id);
  const [restore, setRestore] = useState<BackupRun>(), [files, setFiles] = useState<BackupRun>(), [remove, setRemove] = useState<BackupRun>(), [error, setError] = useState<string>(), [busy, setBusy] = useState(false);
  const disabled = !hostOnline || busy || !!volume.running;
  return <Card className="gap-3 py-4"><CardHeader><CardTitle>{t("history")}</CardTitle></CardHeader><CardContent className="space-y-3">
    {volume.backupUnavailable && <p role="status">{t("hostUnavailable")}</p>}{error && <p role="alert" className="text-sm text-destructive">{error}</p>}
    {!volume.backups.length && !volume.backupUnavailable && <p className="text-sm text-muted-foreground">{t("historyEmpty")}</p>}
    <ul className="space-y-3">{volume.backups.map((run) => <li key={run.id} className="space-y-2 rounded-xl border p-3" data-testid={`backup-${run.id}`}>
      <div className="flex flex-wrap items-center justify-between gap-2"><span className="text-sm">{formatDateTime(run.startedAt)}</span><ToneBadge tone={STATUS_TONE[run.status]}>{t(`status.${run.status}`)}</ToneBadge></div>
      <p className="text-xs text-muted-foreground">{t(`trigger.${run.trigger}`)} · {targets.find((target) => target.id === run.targetId)?.name ?? run.targetId} · {run.sizeBytes === undefined ? "–" : formatBytes(run.sizeBytes)}</p>
      {run.errorCode && <p className="text-sm text-destructive">{all(backupCodeKey(run.errorCode) ?? "common.commandErrors.failed")}</p>}
      {run.archive && <p className="break-all font-mono text-xs text-muted-foreground">{run.archive}</p>}
      <div className="flex flex-wrap gap-2">
        {(run.status === "succeeded" || run.status === "partial") && <><Button variant="outline" size="sm" disabled={disabled || !restoreCapability.available} title={restoreCapability.reason} onClick={() => setRestore(run)}>{t("restore")}</Button><Button variant="outline" size="sm" disabled={disabled || !filesCapability.available || volume.volumeExists === false} title={filesCapability.reason} onClick={() => setFiles(run)}>{t("restoreFiles")}</Button><Button variant="outline" size="sm" disabled={disabled || !downloadCapability.available} title={downloadCapability.reason} onClick={async () => { setBusy(true); setError(undefined); try { const blob = await adapter.downloadBackup(volume.id, run.id); const url = URL.createObjectURL(blob); const link = document.createElement("a"); link.href = url; link.download = `${volume.name}-${run.id}.tar.gz`; link.click(); URL.revokeObjectURL(url); } catch (cause) { setError(backupErrorMessage(cause, all)); } finally { setBusy(false); } }}>{t("download")}</Button></>}
        {run.status !== "running" && <Button variant="outline" size="sm" disabled={disabled || !deleteCapability.available} title={deleteCapability.reason} onClick={() => setRemove(run)}>{t("delete")}</Button>}
      </div>
    </li>)}</ul>
  </CardContent>
    {restore && <RestoreDialog key={restore.id} volume={volume} runId={restore.id} onClose={() => setRestore(undefined)} accepted={accepted} />}
    {files && <RestoreFilesSheet key={files.id} volumeId={volume.id} runId={files.id} onClose={() => setFiles(undefined)} accepted={accepted} />}
    <ConfirmDialog open={!!remove} onOpenChange={(open) => !open && setRemove(undefined)} title={t("delete")} description={t("deleteHint")} confirmLabel={t("delete")} onConfirm={async () => { const feedback = captureCommandFailure(adapter); try { await adapter.deleteBackup(volume.id, remove!.id); } catch (cause) { const message = backupErrorMessage(cause, all); setError(message); feedback.error(message); return false; } }} />
  </Card>;
}
