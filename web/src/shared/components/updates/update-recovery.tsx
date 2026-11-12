// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { useTranslations } from "use-intl";
import { RECOVERY_OUTCOME_KEYS } from "@/data/wire-catalogue-update";
import { useImageError } from "@/features/images/use-image-error";
import type { ActionRun } from "@/domain/model";

export function UpdateRecovery({ run, volumeWarning = true }: { run: ActionRun; volumeWarning?: boolean }) {
  const t = useTranslations(), errors = useImageError();
  const recovery = run.updateRecovery;
  if (!recovery) return null;
  return <div role="status" data-update-recovery={recovery.outcome} className="rounded-lg border p-3 text-sm">
    <p>{t(RECOVERY_OUTCOME_KEYS[recovery.outcome])}</p>
    {recovery.code && <p>{errors(recovery.code)}</p>}
    {volumeWarning && (recovery.outcome !== "not-needed" || run.status === "failed") ? <p className="text-xs text-muted-foreground">{t("common.stackUpdates.volumeNotRestored")}</p> : null}
  </div>;
}
