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
  /** Producer count of image references absent on the host. */
  notPulled?: number;
  /** End of the last successful update run. */
  updatedAt?: string;
}

/**
 * Producer `gaps` (stacks) name images absent on the host. When they are the stack's only gap, the stack-wide
 * incomplete flag stands for them and no longer counts services as unchecked.
 */
export function checkTimes(updates: UpdateInfo[], gaps?: UpdateGaps): Pick<UpdateTimes, "checkedAt" | "unchecked" | "notPulled"> {
  let checkedAt: string | undefined;
  let unchecked = 0;
  const onlyNotPulled = !!gaps && gaps.missing === 0 && gaps.notPulled > 0;
  for (const u of updates) {
    if (!("checkedAt" in u) || !u.checkedAt) {
      unchecked++;
      continue;
    }
    if (u.incomplete && !onlyNotPulled) unchecked++;
    if (!checkedAt || u.checkedAt > checkedAt) checkedAt = u.checkedAt;
  }
  return { checkedAt, unchecked, ...(gaps?.notPulled ? { notPulled: gaps.notPulled } : {}) };
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
