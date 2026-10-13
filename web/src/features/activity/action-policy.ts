import type { ActionKind, ActionRun } from "@/domain/model";

/**
 * Actions that change what runs or what is stored: they get the centred
 * action dialog in addition to their toast. Starts, stops, checks, scans and
 * small saves stay toast-only.
 */
export const MAJOR_ACTION_KINDS: ReadonlySet<ActionKind> = new Set<ActionKind>(["deploy", "redeploy", "update", "restore", "volume-restore", "backup", "pull", "prune", "recreate"]);

export function isMajorAction(run: Pick<ActionRun, "kind">): boolean {
  return MAJOR_ACTION_KINDS.has(run.kind);
}

/** Stack actions that belong to a stack's deploy history. */
export const DEPLOY_KINDS: ReadonlySet<ActionKind> = new Set<ActionKind>(["deploy", "redeploy", "update", "restore"]);
