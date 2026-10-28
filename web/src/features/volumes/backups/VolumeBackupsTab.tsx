// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original live backup tab, failed-write alert and running-operation rediscovery.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useBackupOverview } from "@/data/backup-queries";
import { BackupError, backupErrorMessage } from "@/data/backup-error";
import { useAdapter } from "@/data/use-adapter";
import type { VolumeDetail } from "@/domain/volumes";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { Field, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Switch } from "@/shared/ui/switch";
import { useVolumeCapability } from "../use-volume-capability";
import { BackupHistory } from "./BackupHistory";
import { BackupPolicyCard } from "./BackupPolicyCard";
import { BackupOperation } from "./BackupOperation";
import { RestoreDialog } from "./RestoreDialog";

export function VolumeBackupsTab({ volume, hostOnline }: { volume: VolumeDetail; hostOnline: boolean }) {
  const t = useTranslations("volumes.backups");
  if (volume.kind === "bind") return <p role="status">{t("namedVolumesOnly")}</p>;
  return (
    <CapabilityGate operation="getBackupOverview" resourceRef={{ kind: "host", id: volume.hostId }}>
      <ConnectedTab key={volume.id} volume={volume} hostOnline={hostOnline} />
    </CapabilityGate>
  );
}
function ConnectedTab({ volume, hostOnline }: { volume: VolumeDetail; hostOnline: boolean }) {
  const overview = useBackupOverview(volume.hostId),
    t = useTranslations("volumes.backups"),
    all = useTranslations(),
    adapter = useAdapter();
  const backup = useVolumeCapability("runBackup", volume.id),
    upload = useVolumeCapability("uploadRestore", volume.id);
  const [stop, setStop] = useState(false),
    [activity, setActivity] = useState<string>(),
    [file, setFile] = useState<File>(),
    [safety, setSafety] = useState<string>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>();
  if (overview.isError) return <QueryError error={overview.error} onRetry={() => void overview.refetch()} />;
  if (!overview.data) return <p role="status">{t("following")}</p>;
  const observed = overview.data.volumes.find((v) => v.id === volume.id);
  const current = { ...volume, ...observed, backups: volume.backups, consumers: volume.consumers };
  const running = observed?.running?.activityId;
  // Oldest first: its safety snapshot predates every failed write that is still pending.
  const pending = [...(observed?.pendingRestores ?? [])].reverse();
  const stopped = [
    ...new Map(pending.flatMap((run) => run.stoppedContainers ?? []).map((c) => [c.id, c.name])).values(),
  ];
  const safetyId = pending.find((run) => run.safetyBackupId)?.safetyBackupId;
  return (
    <>
      {current.volumeExists === false && <p role="status">{t("volumeMissing")}</p>}
      {pending.length > 0 && (
        <div role="alert" className="space-y-2 rounded-xl border border-destructive p-4 text-sm">
          <p>{t("failedRestore", { names: stopped.join(", ") || "–", safety: safetyId ?? "–" })}</p>
          {safetyId && (
            <Button variant="outline" onClick={() => setSafety(safetyId)}>
              {t("restoreSafety")}
            </Button>
          )}
        </div>
      )}
      {(running || activity) && (
        <BackupOperation key={running ?? activity} hostId={volume.hostId} activityId={(running ?? activity)!} />
      )}
      <div className="flex flex-wrap items-end gap-3 rounded-xl border p-4">
        <Field orientation="horizontal" className="w-auto">
          <Switch id="backup-now-stop" checked={stop} onCheckedChange={setStop} />
          <FieldLabel htmlFor="backup-now-stop">{t("stop")}</FieldLabel>
        </Field>
        <Button
          disabled={!hostOnline || busy || !!running || !backup.available}
          title={backup.reason}
          onClick={async () => {
            setBusy(true);
            setError(undefined);
            try {
              setActivity(await adapter.runBackup(volume.id, { stopContainers: stop }));
            } catch (cause) {
              setError(backupErrorMessage(cause, all));
              if (cause instanceof BackupError && cause.problem.activityId) setActivity(cause.problem.activityId);
              void overview.refetch();
            } finally {
              setBusy(false);
            }
          }}
        >
          {t("runNow")}
        </Button>
        <Field className="w-full sm:w-auto">
          <FieldLabel htmlFor="backup-upload">{t("uploadRestore")}</FieldLabel>
          <Input
            id="backup-upload"
            type="file"
            accept=".tar.gz,.tgz"
            disabled={!hostOnline || busy || !!running || !upload.available}
            onChange={(event) => {
              setFile(event.target.files?.[0]);
              event.target.value = "";
            }}
          />
          <p className="text-xs text-muted-foreground">{t("uploadHint")}</p>
        </Field>
      </div>
      {error && (
        <p role="alert" className="text-sm text-destructive">
          {error}
        </p>
      )}
      <BackupPolicyCard
        key={JSON.stringify(current.backup ?? null)}
        volume={current}
        targets={overview.data.targets}
        hostOnline={hostOnline}
      />
      <BackupHistory volume={current} targets={overview.data.targets} hostOnline={hostOnline} accepted={setActivity} />
      {safety && (
        <RestoreDialog volume={current} runId={safety} onClose={() => setSafety(undefined)} accepted={setActivity} />
      )}
      {file && (
        <RestoreDialog
          key={file.name}
          volume={current}
          file={file}
          onClose={() => setFile(undefined)}
          accepted={setActivity}
        />
      )}
    </>
  );
}
