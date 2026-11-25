// Source: docklet-next/issues/626
// Revision: 505d1715fdf1dc1668655e9affef78b67620f7d9
// Retrieved: 2026-11-25
// License: Apache-2.0
// Deviations: Original bulk check presentation and producer-shaped regression coverage.

import { useTranslations } from "use-intl";
import type { ActionRun, BulkCheckCounts } from "@/domain/model";

type CountText = (key: "checked" | "soFar" | "checkedUnknown" | "updates" | "failed" | "skipped" | "separator", values?: { count: number; planned?: number }) => string;

/** The checked part counts successes while running and at the end, so it never drops at settlement. */
export function formatBulkCheckCounts(bulk: BulkCheckCounts, running: boolean, t: CountText): string {
  const count = bulk.succeeded;
  const parts: string[] = [];
  if (running || count > 0 || bulk.failed + bulk.skipped + bulk.updates === 0) {
    parts.push(t(bulk.plannedKnown ? "checked" : running ? "soFar" : "checkedUnknown", { count, planned: bulk.planned }));
  }
  if (!running && bulk.updates > 0) parts.push(t("updates", { count: bulk.updates }));
  if (bulk.failed > 0) parts.push(t("failed", { count: bulk.failed }));
  if (bulk.skipped > 0) parts.push(t("skipped", { count: bulk.skipped }));
  return parts.join(t("separator"));
}

export function useBulkCheckCounts(run: ActionRun): string | undefined {
  const t = useTranslations("activity.bulk");
  return run.bulk ? formatBulkCheckCounts(run.bulk, run.status === "running" || run.status === "queued", t) : undefined;
}
