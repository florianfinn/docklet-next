// Source: docklet-next/issues/680
// Revision: e962f0fe4ab031f8e375d54e9988de3f1c1e8209
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original minimum image age web consumer and regression coverage; age shown only for scheduled installs with a minimum age.

import { useQuery } from "@tanstack/react-query";
import { useFormatter, useTranslations } from "use-intl";
import type { StackUpdatePolicy } from "@/domain/stack-updates";
import { useAdapter } from "@/data/use-adapter";

/** Shared candidate status for stack and standalone-container policies. */
export function PolicyCandidates({ policy, timeZone }: { policy: StackUpdatePolicy; timeZone: string }) {
  const t = useTranslations("common.stackUpdates"), format = useFormatter(), adapter = useAdapter();
  const settings = useQuery({ queryKey: ["settings", "updates"], queryFn: () => adapter.getUpdateSettings!(), enabled: policy.mode === "scheduled" && !!adapter.getUpdateSettings });
  // Age applies only to scheduled installs with the hub setting above 0; an unreadable setting shows no age.
  const aged = policy.mode === "scheduled" && (settings.data?.minimumAgeDays ?? 0) > 0;
  return <>{policy.candidates.map((candidate) => <p key={`${candidate.service}:${candidate.digest}`}>
    {candidate.service}: {candidate.version || candidate.imageRef} · {t(`notification.${candidate.notificationState}`)}
    {!aged ? null : !candidate.manifest ? <> · {t("ageUnknown")}</> : candidate.eligibleAt ? <> · {t("waitsUntil", { date: format.dateTime(new Date(candidate.eligibleAt), { timeZone, dateStyle: "medium", timeStyle: "short" }) })}</> : null}
  </p>)}</>;
}
