import type { ActionRun } from "@/domain/model";

import { DEPLOY_KINDS } from "./action-policy";

/** Live and recorded deploy runs of a stack, newest first, each once. */
export function deployRuns(live: readonly ActionRun[], recorded: readonly ActionRun[], stackId: string): ActionRun[] {
  const runs = live.filter((r) => r.stackId === stackId && DEPLOY_KINDS.has(r.kind));
  for (const r of recorded) if (!runs.some((x) => x.id === r.id)) runs.push(r);
  return runs.sort((a, b) => b.startedAt.localeCompare(a.startedAt));
}

/** A run picked from the history, remembered with the newest run at that moment. */
export interface HistoryPick {
  runId: string;
  latestId: string;
}

/** The run in the main view: the pick until a newer run starts, then the newest one. */
export function shownRun(runs: readonly ActionRun[], pick: HistoryPick | null): ActionRun | undefined {
  const latest = runs[0];
  if (!latest) return undefined;
  if (pick && pick.latestId === latest.id) return runs.find((r) => r.id === pick.runId) ?? latest;
  return latest;
}
