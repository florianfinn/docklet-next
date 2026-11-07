// Source: docklet-next/issues/341
// Revision: 593699500ef01a82fef720cc7217ed19eaaae7c0
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A stack run observed going terminal refreshes the stack inventory, whether the change came from the stream, a receipt poll or an activity read. M20 #360: shell list reads and target-specific terminal invalidation.

const ACTIVE = new Set(["queued", "running", "waiting", "retrying"]);

export interface StackTarget { stackId: string; hostId: string }

export function createStackTerminals(ended: (target: StackTarget) => void) {
  const active = new Set<string>();
  const delivered = new Map<string, string>();
  const observe = (runs: readonly { id: string; hostId?: string; stackId?: string; status: string; finishedAt?: string }[], confirmed = false) => {
    for (const run of runs) {
      if (!run.stackId || !run.hostId) continue;
      const key = JSON.stringify([run.hostId, run.id]);
      if (ACTIVE.has(run.status)) { active.add(key); continue; }
      const wasActive = active.delete(key);
      const stamp = `${run.status}:${run.finishedAt ?? ""}`;
      // A historical terminal list row is a baseline, not a newly completed command.
      if ((!wasActive && !confirmed) || delivered.get(key) === stamp) continue;
      delivered.set(key, stamp);
      ended({ stackId: run.stackId, hostId: run.hostId });
    }
  };
  observe.reset = () => { active.clear(); delivered.clear(); };
  return observe;
}
