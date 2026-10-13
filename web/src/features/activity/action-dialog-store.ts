import { createStore } from "@/shared/lib/store";

export interface ActionDialogState {
  /** Run shown in the centred action dialog; undefined while it is closed. */
  runId?: string;
  /** Major runs that started while another run was shown; offered, never switched to by themselves. */
  waiting: string[];
}

const CLOSED: ActionDialogState = { waiting: [] };

/** Deliberate open (toast, action centre, offer in the dialog): shows exactly this run. */
export function showRun(state: ActionDialogState, runId: string): ActionDialogState {
  return { runId, waiting: state.waiting.filter((id) => id !== runId) };
}

/** Automatic open for a new major run: an open dialog keeps its run and only offers the new one. */
export function offerRun(state: ActionDialogState, runId: string): ActionDialogState {
  if (!state.runId) return showRun(state, runId);
  if (state.runId === runId || state.waiting.includes(runId)) return state;
  return { ...state, waiting: [...state.waiting, runId] };
}

/** Closing only hides the dialog; the action keeps running and its toast stays. */
export const actionDialogStore = createStore<ActionDialogState>(CLOSED);

export function openActionDialog(runId: string): void {
  actionDialogStore.set((s) => showRun(s, runId));
}

export function offerActionDialog(runId: string): void {
  actionDialogStore.set((s) => offerRun(s, runId));
}

export function closeActionDialog(): void {
  actionDialogStore.set(CLOSED);
}
