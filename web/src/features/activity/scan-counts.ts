// Source: docklet-next/issues/583
// Revision: 343b1df4b249529e427ce1b7e38f1eec74e95187
// Retrieved: 2026-11-23
// License: Apache-2.0
// Deviations: Original activity titles, host targets and scan presentation. M37 #584: truthful activity titles, host targets and scan counts. M39 #612: one-line result of a terminal host scan.

import { useTranslations } from "use-intl";
import type { ActionRun } from "@/domain/model";
import type { ScanRunSummary } from "@/domain/security";

type Translate = ReturnType<typeof useTranslations<"runMessages.scanCounts">>;

/** Host scans own a durable run record; image scans and candidate runs do not use it. */
export const isHostScan = (run: Pick<ActionRun, "kind" | "hostScoped" | "sourceOperation">) =>
  run.kind === "scan" && run.hostScoped === true && run.sourceOperation !== "vulnerability.candidate-scan";

/** Any vulnerability scan run, host, image or candidate; they share one notification. */
export const isScanRun = (run: Pick<ActionRun, "kind" | "sourceOperation">) => run.kind === "scan" || run.sourceOperation === "vulnerability.candidate-scan";

/**
 * A failed or partial scan result stays until closed (#611). The durable record can be partial under a succeeded
 * activity: a host run with failed images, or a candidate run with failed targets. Deferred targets only hit the
 * per-run limit and wait for later scans, so they keep the ordinary lifetime.
 */
export const scanPersistent = (run: ActionRun) =>
  isScanRun(run) && (run.status === "failed" || ["partial", "failed", "interrupted"].includes(run.scan?.summary?.status ?? "")
    || (run.candidateScan !== undefined && run.candidateScan.failed > 0));

/**
 * The copy a scan notification classifies: a failed detail refresh that kept a non-terminal copy yields to the
 * settled listed run (its outcome is known, its result is not), so the ordinary lifetime applies.
 */
export const classifiedCopy = (listed: ActionRun, detail: ActionRun, failed: boolean) =>
  failed && (detail.status === "running" || detail.status === "queued") && listed.status !== "running" && listed.status !== "queued" ? listed : detail;

/** "6 images scanned · 2 critical · 15 high · 56 s", plus failed images and a partial marker when they apply. */
export function scanResultLine(summary: ScanRunSummary, t: Translate): string {
  const ms = summary.finishedAt ? Date.parse(summary.finishedAt) - Date.parse(summary.startedAt) : NaN;
  const seconds = Number.isFinite(ms) && ms >= 0 ? Math.round(ms / 1000) : undefined;
  const duration = seconds === undefined ? "" : seconds < 120 ? t("seconds", { seconds }) : t("minutes", { minutes: Math.round(seconds / 60) });
  const main = summary.targets === 0 ? t("empty") : t("result", { scanned: summary.scanned, critical: summary.counts.critical, high: summary.counts.high });
  return [main, summary.failed > 0 ? t("failed", { failed: summary.failed }) : "", summary.status === "partial" ? t("partial") : "", duration].filter(Boolean).join(" · ");
}

export function useScanResultLine(summary: ScanRunSummary | undefined): string | undefined {
  const t = useTranslations("runMessages.scanCounts");
  return summary ? scanResultLine(summary, t) : undefined;
}

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
  if (counts?.summary && run.status !== "running" && run.status !== "queued") return scanResultLine(counts.summary, t);
  if (!counts || counts.targets === 0) return undefined;
  const pct = Math.min(100, Math.round(100 * counts.done / counts.targets));
  const { targets, done, failed } = counts;
  return [t("manual", { targets, done, pct }), failed > 0 ? t("failed", { failed }) : ""].filter(Boolean).join(" · ");
}
