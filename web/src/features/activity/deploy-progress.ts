// Source: docklet-next/issues/567
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original executor-owned deploy progress observation and behavioural coverage.

import { useTranslations } from "use-intl";
import { formatBytes } from "@/domain/format";
import type { ActionRun } from "@/domain/model";

export interface DeployProgress {
  phase: "pull" | "deploy";
  pull?: { images: number; layersDone: number; layersTotal: number; bytesDone?: number; bytesTotal?: number };
  activity?: { label: "create" | "stop" | "start" | "wait"; services: number };
}
export type DeployProgressView<T> = T & { deployProgress?: DeployProgress };

/** The mapper supplies an atomic projection of the row owning this status. */
export function useDeployProgress(run: ActionRun): string | undefined {
  const t = useTranslations("activity.deployProgress");
  const progress = (run as DeployProgressView<ActionRun>).deployProgress;
  if (run.status !== "running" || run.outcomeState === "unknown" || !progress) return undefined;
  const parts = [t(progress.phase)];
  if (progress.phase === "pull" && progress.pull) {
    const pull = progress.pull;
    parts.push(t("layers", { done: pull.layersDone, total: pull.layersTotal }));
    if (pull.bytesDone !== undefined && pull.bytesTotal !== undefined) parts.push(t("bytes", { done: formatBytes(pull.bytesDone), total: formatBytes(pull.bytesTotal) }));
  }
  if (progress.phase === "deploy" && progress.activity) parts.push(t(progress.activity.label, { count: progress.activity.services }));
  return parts.join(" · ");
}
