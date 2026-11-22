// Source: docklet-next/issues/571
// Revision: 723ece0231761c87611eef58d8f7e0a44c759ea1
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original repository check stage and result presentation shared by run focus and backup operation line.

import { useTranslations } from "use-intl";
import type { ActionRun, StepState } from "@/domain/model";

type CheckRun<L> = Pick<ActionRun, "kind" | "status" | "repositoryCheck"> & { steps: readonly { state: StepState; label: L }[] };

/**
 * A repository check reaches a variable number of stages, so it names its current stage, failing stage or
 * validated result instead of a step count; undefined for other runs and for states without such a fact.
 */
export function useRepositoryCheckText<L>(run: CheckRun<L> | undefined, label: (value: L) => string): string | undefined {
  const t = useTranslations();
  if (run?.kind !== "repository-check") return undefined;
  const reached = [...run.steps].reverse();
  if (run.status === "running") {
    const step = reached.find((s) => s.state === "running");
    return step ? label(step.label) : undefined;
  }
  if (run.status === "succeeded") return run.repositoryCheck ? t("volumes.backups.repositoryCheck.result", { count: run.repositoryCheck.snapshotCount }) : undefined;
  if (run.status === "failed") {
    const step = reached.find((s) => s.state === "error") ?? reached.find((s) => s.state !== "pending" && s.state !== "skipped");
    return step ? t("activity.failedAt", { label: label(step.label) }) : undefined;
  }
  return undefined;
}
