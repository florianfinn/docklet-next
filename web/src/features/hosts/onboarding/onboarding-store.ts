import type { HostConnection } from "@/domain/model";
import { createStore, useStore } from "@/shared/lib/store";

export interface HostDraft {
  name: string;
  color: string;
  connection: HostConnection;
  address: string;
  port: string;
  projectDir: string;
  installDir: string;
  manualIds: boolean;
  puid: string;
  pgid: string;
  detectSocket: boolean;
  socketGid: string;
  token?: string;
  /** Set once the host is registered; earlier steps become read-only. */
  hostId?: string;
  runId?: string;
  simulateFailure: boolean;
  /** Highest step index reached, for the step rail. */
  reached: number;
}

/** Draft of the running onboarding; in memory only, survives navigation inside the app. */
export const onboardingStore = createStore<HostDraft | null>(null);

export function useOnboardingDraft(): HostDraft | null {
  return useStore(onboardingStore, (d) => d);
}

export function patchDraft(patch: Partial<HostDraft>): void {
  onboardingStore.set((d) => (d ? { ...d, ...patch } : d));
}

