// Source: docklet-next/issues/680
// Revision: e962f0fe4ab031f8e375d54e9988de3f1c1e8209
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age web consumer and regression coverage.

import { useFormatter, useTranslations } from "use-intl";
import type { StackUpdatePolicy } from "@/domain/stack-updates";

/** Shared candidate status for stack and standalone-container policies. */
export function PolicyCandidates({ policy, timeZone }: { policy: StackUpdatePolicy; timeZone: string }) {
  const t = useTranslations("common.stackUpdates"), format = useFormatter();
  return <>{policy.candidates.map((candidate) => <p key={`${candidate.service}:${candidate.digest}`}>
    {candidate.service}: {candidate.version || candidate.imageRef} · {t(`notification.${candidate.notificationState}`)}
    {!candidate.manifest ? <> · {t("ageUnknown")}</> : candidate.eligibleAt ? <> · {t("waitsUntil", { date: format.dateTime(new Date(candidate.eligibleAt), { timeZone, dateStyle: "medium", timeStyle: "short" }) })}</> : null}
  </p>)}</>;
}
