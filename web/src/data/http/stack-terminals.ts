// Source: docklet-next/issues/341
// Revision: 593699500ef01a82fef720cc7217ed19eaaae7c0
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: A stack run observed going terminal refreshes the stack inventory, whether the change came from the stream, a receipt poll or an activity read.

const ACTIVE = new Set(["queued", "running", "waiting", "retrying"]);

export function createStackTerminals(ended: () => void) {
  const active = new Set<string>();
  return (runs: readonly { id: string; hostId?: string; stackId?: string; status: string }[]) => {
    let changed = false;
    for (const run of runs) {
      if (!run.stackId) continue;
      const key = `${run.hostId ?? ""}/${run.id}`;
      if (ACTIVE.has(run.status)) active.add(key);
      else if (active.delete(key)) changed = true;
    }
    if (changed) ended();
  };
}
