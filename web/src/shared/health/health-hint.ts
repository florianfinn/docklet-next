// Source: docklet-next/issues/637
// Revision: 24638d9cd8ce431af9ee9b829ec68b171c77e287
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 shared health hint text.

import { useTranslations } from "use-intl";
import type { HealthDiagnosis } from "@/domain/health";

/** One localized sentence per reason; healthy has none. */
export function useHealthHint(): (diagnosis: HealthDiagnosis | undefined) => string | null {
  const t = useTranslations("health");
  return (diagnosis) => !diagnosis || diagnosis.reason === "healthy" ? null : diagnosis.reason === "http-status"
    ? diagnosis.httpStatus === 404 ? t("reasons.http404") : t("reasons.http-status", { status: diagnosis.httpStatus ?? "–" })
    : diagnosis.reason === "tool-missing" ? t("reasons.tool-missing", { tool: diagnosis.tool ?? "–" }) : t(`reasons.${diagnosis.reason}`);
}
