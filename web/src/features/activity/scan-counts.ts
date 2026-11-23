// Source: docklet-next/issues/583
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts.

import { useTranslations } from "use-intl";
import type { ActionRun } from "@/domain/model";

/** Candidate counters describe stored outcomes, not an exhaustive checked numerator. */
export function useScanCounts(run: ActionRun): string | undefined {
  const t = useTranslations("runMessages.scanCounts"), activity = useTranslations("activity.status");
  if (run.sourceOperation === "vulnerability.candidate-scan") {
    if (run.status === "running" || run.status === "queued") return activity(run.status);
    const counts = run.candidateScan;
    if (!counts) return undefined;
    return [t("candidate", counts), counts.deferred > 0 ? t("deferred", counts) : "", counts.failed > 0 ? t("failed", counts) : ""].filter(Boolean).join(" · ");
  }
  const counts = run.scan;
  if (!counts || counts.targets === 0) return undefined;
  const pct = Math.min(100, Math.round(100 * counts.done / counts.targets));
  return [t("manual", { ...counts, pct }), counts.failed > 0 ? t("failed", counts) : ""].filter(Boolean).join(" · ");
}
