// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original per-host local repository status with read-only engine image.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { useBackupRepository } from "@/data/backup-queries";
import { backupCodeKey, backupErrorMessage } from "@/data/backup-error";
import { useAdapter } from "@/data/use-adapter";
import type { Host } from "@/domain/model";
import { BackupOperation } from "@/features/volumes/backups/BackupOperation";
import { Button } from "@/shared/ui/button";

export function RepositoryCard({ host, recovery }: { host: Host; recovery: () => void }) {
  const adapter = useAdapter(),
    repository = useBackupRepository(host.id),
    t = useTranslations("settings.backups"),
    all = useTranslations();
  const [activity, setActivity] = useState<string>(),
    [busy, setBusy] = useState(false),
    [error, setError] = useState<string>();
  const value = repository.data;
  return (
    <article className="space-y-3 rounded-xl border p-4">
      <h3 className="font-medium">
        {t("builtInLocal")} · {host.name}
      </h3>
      {repository.isError ? (
        <p role="status">{t("unavailable")}</p>
      ) : !value ? (
        <p role="status">{t("loading")}</p>
      ) : (
        <>
          <p className="break-all text-sm">
            {value.mode} · {value.name}
          </p>
          <dl className="text-xs">
            <dt>{t("engineImage")}</dt>
            <dd className="break-all font-mono">{value.engineImage}</dd>
          </dl>
          <p className="text-sm">
            {value.lastCheck
              ? `${value.lastCheck.ok ? t("testOk") : t("testFailed")} · ${new Date(value.lastCheck.at).toLocaleString()}`
              : t("untested")}
          </p>
          {value.lastCheck?.code && <p>{all(backupCodeKey(value.lastCheck.code) ?? "common.commandErrors.failed")}</p>}
        </>
      )}
      <div className="flex flex-wrap gap-2">
        <Button
          variant="outline"
          disabled={busy || host.status !== "online" || !!value?.running}
          onClick={async () => {
            setBusy(true);
            try {
              setActivity(await adapter.checkBackupRepository(host.id));
            } catch (cause) {
              setError(backupErrorMessage(cause, all));
            } finally {
              setBusy(false);
            }
          }}
        >
          {t("checkRepository")}
        </Button>
        <Button variant="outline" onClick={recovery}>
          {t("recoveryKey")}
        </Button>
      </div>
      {(value?.running?.activityId || activity) && (
        <BackupOperation hostId={host.id} activityId={(value?.running?.activityId ?? activity)!} />
      )}
      {error && <p role="alert">{error}</p>}
    </article>
  );
}
