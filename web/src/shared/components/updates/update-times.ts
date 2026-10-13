import { useActionRuns } from "@/data/live";
import type { ActionRun, StackDetail, UpdateInfo } from "@/domain/model";

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
  /** End of the last successful update run. */
  updatedAt?: string;
}

export function checkTimes(updates: UpdateInfo[]): Pick<UpdateTimes, "checkedAt" | "unchecked"> {
  let checkedAt: string | undefined;
  let unchecked = 0;
  for (const u of updates) {
    if (!("checkedAt" in u)) {
      unchecked++;
      continue;
    }
    if (!checkedAt || u.checkedAt > checkedAt) checkedAt = u.checkedAt;
  }
  return { checkedAt, unchecked };
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

export function stackUpdateTimes(stack: Pick<StackDetail, "id" | "services" | "deployments">, live: Pick<ActionRun, "kind" | "status" | "finishedAt" | "stackId">[]): UpdateTimes {
  return { ...checkTimes(stack.services.map((s) => s.update)), updatedAt: lastUpdateAt([...stack.deployments, ...live], stack.id) };
}

export function useStackUpdateTimes(stack: Pick<StackDetail, "id" | "services" | "deployments">): UpdateTimes {
  return stackUpdateTimes(stack, useActionRuns());
}
