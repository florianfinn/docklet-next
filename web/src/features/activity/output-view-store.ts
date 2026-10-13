import type { ActionRun } from "@/domain/model";
import { createStore, useStore } from "@/shared/lib/store";

export interface OutputView {
  open: boolean;
  onlyStderr: boolean;
}

/** Raw-output view state per action run, shared by toast dialog, action centre and deploy history. */
export const outputViewStore = createStore<Record<string, OutputView>>({});

export function useOutputView(runId: string, fallback: OutputView): [OutputView, (patch: Partial<OutputView>) => void] {
  const stored = useStore(outputViewStore, (s) => s[runId]);
  const view = stored ?? fallback;
  const update = (patch: Partial<OutputView>) => outputViewStore.set((s) => ({ ...s, [runId]: { ...(s[runId] ?? fallback), ...patch } }));
  return [view, update];
}

/** View before anyone toggled it: open when the surface asks for it or the run failed. */
export function outputFallback(run: Pick<ActionRun, "status">, defaultOpen: boolean): OutputView {
  return { open: defaultOpen || run.status === "failed", onlyStderr: false };
}

/** Whether a run's raw output is open; surfaces that size themselves around it read this. */
export function useOutputOpen(run: Pick<ActionRun, "id" | "status">, defaultOpen: boolean): boolean {
  return useStore(outputViewStore, (s) => s[run.id]?.open) ?? outputFallback(run, defaultOpen).open;
}
