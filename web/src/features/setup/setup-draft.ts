import { defaultModules, type ModuleChoices } from "@/domain/modules";
import { hubAddressCandidates, type HubDiscovery } from "@/domain/hub-discovery";
import type { AuthSettings, GeneralSettings, HostDefaults } from "@/domain/settings";
import type { PasswordDraft } from "@/features/settings/field-rules";
import { createStore, useStore } from "@/shared/lib/store";

export const SETUP_STEPS = ["access", "hub", "projects", "modules", "review"] as const;
export type SetupStep = (typeof SETUP_STEPS)[number];

/** Where the internal hub address came from; discovery never replaces a manual value. */
export type HubAddressOrigin = { kind: "discovered"; candidateId: string } | { kind: "manual" } | null;

export interface SetupDraft {
  modules: ModuleChoices;
  general: GeneralSettings;
  hubAddressOrigin: HubAddressOrigin;
  hostDefaults: HostDefaults;
  auth: AuthSettings;
  /** In memory only; cleared on finish and never sent to the adapter. */
  password: PasswordDraft;
  reached: number;
  /** Connected wizard took over the hub's saved values (upgraded or resumed hub). */
  hydrated: boolean;
}

/** First-run values of a fresh hub (not the current demo settings). */
export function freshSetupDraft(): SetupDraft {
  return {
    modules: defaultModules(),
    general: { hubName: "Docklet", hubAddress: "", domainUrl: "", linkBase: "auto", timeZone: "Europe/Berlin" },
    hubAddressOrigin: null,
    hostDefaults: { projectDir: "/home/docker", installDir: "/home/docker/docklet-agent", connection: "direct", puid: "", pgid: "", detectSocketGroup: true },
    auth: {
      mode: "local",
      local: { username: "admin", passwordSet: false },
      proxy: { userHeader: "Remote-User", emailHeader: "Remote-Email", groupsHeader: "Remote-Groups", trustedProxies: "", logoutUrl: "" },
    },
    password: { password: "", confirm: "" },
    reached: 0,
    hydrated: false,
  };
}

export const setupStore = createStore<SetupDraft>(freshSetupDraft());

export function useSetupDraft(): SetupDraft {
  return useStore(setupStore, (d) => d);
}

export function patchSetup(patch: Partial<SetupDraft>): void {
  setupStore.set((d) => ({ ...d, ...patch }));
}

/** Design lab entry point: replay the wizard from a clean draft. */
export function restartSetup(): void {
  setupStore.set(freshSetupDraft());
}

/**
 * Prefills the internal hub address with the recommended discovery candidate.
 * Runs whenever discovery data arrives; a value the user typed or cleared
 * (origin "manual") and any already filled address stay untouched.
 */
export function applyDiscovery(draft: SetupDraft, discovery: HubDiscovery): SetupDraft {
  if (draft.hubAddressOrigin !== null || draft.general.hubAddress.trim()) return draft;
  const best = hubAddressCandidates(discovery).find((c) => c.recommended);
  if (!best) return draft;
  return { ...draft, general: { ...draft.general, hubAddress: best.address }, hubAddressOrigin: { kind: "discovered", candidateId: best.id } };
}

/**
 * A hub that already holds addresses (upgraded or resumed) starts the wizard from its saved values, so
 * Finish never resets them to first-run defaults. A fresh hub keeps the first-run draft.
 */
export function hydrateFromSettings(draft: SetupDraft, saved: { general: GeneralSettings; hostDefaults: HostDefaults }, modules?: ModuleChoices): SetupDraft {
  if (draft.hydrated) return draft;
  if (!saved.general.hubAddress.trim() && !saved.general.domainUrl.trim()) return { ...draft, hydrated: true };
  return { ...draft, general: { ...saved.general }, hostDefaults: { ...saved.hostDefaults }, ...(modules ? { modules } : {}), hubAddressOrigin: { kind: "manual" }, hydrated: true };
}
