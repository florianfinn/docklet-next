import { defaultModules, type ModuleChoices } from "@/domain/modules";
import { hubAddressCandidates, serverCandidateKind, type HubDiscovery } from "@/domain/hub-discovery";
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
  /** The operator typed in or cleared the domain; discovery then never fills it. */
  domainEdited: boolean;
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
    domainEdited: false,
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
  // Each field is filled on its own, only while empty and untouched (a hub without a reachable IP still has its domain).
  const best = draft.hubAddressOrigin === null && !draft.general.hubAddress.trim() ? hubAddressCandidates(discovery).find((c) => c.recommended) : undefined;
  const domain = draft.domainEdited || draft.general.domainUrl.trim() ? undefined
    : discovery.candidates?.find((c) => serverCandidateKind(c) === "domain")?.url.replace(/\/+$/, "");
  if (!best && !domain) return draft;
  return {
    ...draft,
    general: { ...draft.general, ...(best ? { hubAddress: best.address } : {}), ...(domain ? { domainUrl: domain } : {}) },
    ...(best ? { hubAddressOrigin: { kind: "discovered" as const, candidateId: best.id } } : {}),
  };
}

/**
 * A connected wizard starts from the hub's saved values, so Finish never resets an upgraded hub to first-run
 * defaults; a fresh hub's saved values are those defaults. Once either address is saved both count as typed (a blank
 * domain beside a saved IP is a choice); with neither saved, discovery may fill them.
 */
export function hydrateFromSettings(draft: SetupDraft, saved: { general: GeneralSettings; hostDefaults: HostDefaults }, modules?: ModuleChoices): SetupDraft {
  if (draft.hydrated) return draft;
  const { hubName, hubAddress, domainUrl, linkBase, timeZone } = saved.general;
  const addressed = hubAddress.trim() !== "" || domainUrl.trim() !== "";
  return {
    // Only the wizard's own fields: Finish never writes the time zone or other General settings.
    ...draft, general: { hubName, hubAddress, domainUrl, linkBase, timeZone }, hostDefaults: { ...saved.hostDefaults }, ...(modules ? { modules } : {}),
    hubAddressOrigin: addressed ? { kind: "manual" } : null, domainEdited: addressed, hydrated: true,
  };
}
