// Source: docklet-next/docs/design/bind-mounts.md
// Revision: 6b4bed6974532df3c991cf2f86ae693add35ba52
// Retrieved: 2026-11-02
// License: Apache-2.0
// Deviations: Original notice for bind mounts that the agent refused or has not checked.

import { useTranslations } from "use-intl";

import { backupCodeKey } from "@/data/backup-error";
import { storageUsable, type Volume } from "@/domain/volumes";

/** Why files or backups are restricted for a bind; renders nothing for fully usable entries. */
export function BindNotice({ volume, history = false }: { volume: Pick<Volume, "kind" | "bind">; history?: boolean }) {
  const t = useTranslations("volumes.detail");
  const all = useTranslations();
  if (!history && storageUsable(volume)) return null;
  const reason = backupCodeKey(volume.bind?.reason);
  return (
    <p role="status" className="text-sm text-muted-foreground">
      {history ? t("bindHistoryOnly") : volume.bind?.eligible ? t("bindKindUnknown") : volume.bind ? t("bindDenied") : t("bindUnchecked")}
      {reason && ` ${all(reason)}`}
    </p>
  );
}
