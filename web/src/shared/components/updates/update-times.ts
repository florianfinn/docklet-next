import { useActionRuns } from "@/data/live";
import type { ActionRun, StackDetail, UpdateGaps, UpdateInfo } from "@/domain/model";

/**
 * Times an update panel shows. Every value comes from recorded data: registry
 * checks from the update results, updates from finished update runs in the
 * deploy history. Missing values stay undefined and are labelled as unknown.
 */
export interface UpdateTimes {
  /** Newest registry check across the images. */
  checkedAt?: string;
  /** Images without a registry check (never checked or local only). */
  unchecked: number;
  /** Images absent on the host; counted apart from `unchecked` because the reason is known. */
  notPulled?: number;
  /** End of the last successful update run. */
  updatedAt?: string;
}

/**
 * With producer `gaps` (stacks), image counts come from the producer, which counts distinct references; services
 * only add the newest check and local builds, which cannot be checked. Without gaps, each service counts itself.
 */
export function checkTimes(updates: UpdateInfo[], gaps?: UpdateGaps): Pick<UpdateTimes, "checkedAt" | "unchecked" | "notPulled"> {
  let checkedAt: string | undefined;
  let unchecked = 0, notPulled = 0, local = 0;
  for (const u of updates) {
    if (u.state === "local") local++;
    if (u.notPulled) notPulled++;
    if (!("checkedAt" in u) || !u.checkedAt) {
      unchecked++;
      continue;
    }
    if (u.incomplete && !u.notPulled) unchecked++;
    if (!checkedAt || u.checkedAt > checkedAt) checkedAt = u.checkedAt;
  }
  if (gaps) { unchecked = gaps.missing + local; notPulled = gaps.notPulled; }
  return { checkedAt, unchecked, ...(notPulled ? { notPulled } : {}) };
}

/** End of the newest successful update run of a stack, live runs included. */
export function lastUpdateAt(runs: Pick<ActionRun, "kind" | "status" | "finishedAt" | "stackId">[], stackId: string): string | undefined {
  let last: string | undefined;
  for (const r of runs) {
    if (r.kind !== "update" || r.status !== "succeeded" || r.stackId !== stackId || !r.finishedAt) continue;
    if (!last || r.finishedAt > last) last = r.finishedAt;
  }
  return last;
}

export function stackUpdateTimes(stack: Pick<StackDetail, "id" | "services" | "deployments" | "updateGaps">, live: Pick<ActionRun, "kind" | "status" | "finishedAt" | "stackId">[]): UpdateTimes {
  return { ...checkTimes(stack.services.map((s) => s.update), stack.updateGaps), updatedAt: lastUpdateAt([...stack.deployments, ...live], stack.id) };
}

export function useStackUpdateTimes(stack: Pick<StackDetail, "id" | "services" | "deployments" | "updateGaps">): UpdateTimes {
  return stackUpdateTimes(stack, useActionRuns());
}
