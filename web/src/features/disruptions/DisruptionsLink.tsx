// Source: docklet-next/docs/design/log-health-insights.md
// Revision: 0eb90bfa3ef60a786b02b25aeacad5118e692b40
// Retrieved: 2026-11-28
// License: Apache-2.0
// Deviations: Original disruption timeline and bounded combined log window for #693.

import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useIsUser } from "@/data/use-principal";
import { activityStore } from "@/features/activity/activity-store";
export function DisruptionsLink() {
  const t = useTranslations("disruptions");
  return useIsUser() ? null : <Link className="text-sm underline" to="/disruptions" onClick={() => activityStore.set((s) => ({ ...s, open: false }))}>{t("title")}</Link>;
}
