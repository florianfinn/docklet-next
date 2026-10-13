import { createStore } from "@/shared/lib/store";

/** One Compose service whose container restarts on its own. */
export interface ServiceTarget {
  stackId: string;
  stackName: string;
  service: string;
  /** Container of the service; found from stack and service when absent. Used for configured dependencies. */
  containerId?: string;
}

export interface ServiceRestartRequest {
  targets: ServiceTarget[];
  /** Starts the restart(s); undefined or false means it failed and the dialog stays open for retry. */
  run: () => Promise<unknown>;
}

export const serviceRestartStore = createStore<ServiceRestartRequest | null>(null);

/**
 * Restarting a single service of a Compose stack always asks first: only that
 * container restarts, the rest of the stack keeps running and depends_on is
 * not re-evaluated. Whole-stack and standalone commands never come here.
 */
export function requestServiceRestart(targets: ServiceTarget[], run: () => Promise<unknown>): void {
  if (!targets.length) {
    void run();
    return;
  }
  serviceRestartStore.set({ targets, run });
}
