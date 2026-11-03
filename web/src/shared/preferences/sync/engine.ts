// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original profile preference sync with owner-bound legacy import for issue #339.

import { isUiSection, type UiPreferencesApi, type UiPreferencesDocument, type UiSection } from "@/domain/ui-preferences";

import type { SectionBinding } from "./sections";

export const OWNER_KEY = "docklet-react-opus.preferences-owner";
export const CLAIM_KEY = "docklet-react-opus.legacy-import";

/**
 * Pre-M17 values claimed by the first user who signed in on this browser. The
 * values travel inside the claim, so another user's reset of the stores cannot
 * lose them and only the owner ever uploads them.
 */
interface Claim {
  owner: string;
  pending: Partial<Record<UiSection, object>>;
}

export interface PreferenceSync {
  /** Signed-in principal id, or null while nobody is known. */
  setIdentity(id: string | null): void;
  dispose(): void;
}

interface Options {
  api: UiPreferencesApi;
  bindings: readonly SectionBinding[];
  storage: Storage | undefined;
  debounceMs?: number;
}

export function createPreferenceSync({ api, bindings, storage, debounceMs = 1000 }: Options): PreferenceSync {
  let identity: string | null = null;
  let generation = 0;
  let loaded = false;
  let applying = false;
  const timers = new Map<UiSection, ReturnType<typeof setTimeout>>();
  const dirty = new Set<UiSection>();

  const unsubscribe = bindings.map((b) =>
    b.store.subscribe(() => {
      if (applying || identity === null) return;
      dirty.add(b.section);
      if (loaded) schedule(b);
    }),
  );

  function apply(b: SectionBinding, raw: unknown) {
    applying = true;
    try {
      b.store.set(b.fromWire(raw, b.store.get()));
    } finally {
      applying = false;
    }
  }

  function schedule(b: SectionBinding) {
    clearTimeout(timers.get(b.section));
    timers.set(b.section, setTimeout(() => {
      timers.delete(b.section);
      void send(b);
    }, debounceMs));
  }

  async function send(b: SectionBinding) {
    const owner = identity;
    dirty.delete(b.section);
    // A newer local edit supersedes the claimed legacy value of the same section.
    const claim = readClaim();
    if (claim && claim.owner === owner && b.section in claim.pending) {
      delete claim.pending[b.section];
      writeClaim(claim);
    }
    // Responses are never applied: a late answer cannot undo a newer edit. Failures retry with the next change.
    await api.putUiPreference(b.section, b.toWire(b.store.get())).catch(() => undefined);
  }

  function resetStores() {
    for (const b of bindings) apply(b, undefined);
  }

  function readClaim(): Claim | null {
    try {
      const raw = JSON.parse(storage?.getItem(CLAIM_KEY) ?? "null") as Partial<Claim> | null;
      if (!raw || typeof raw.owner !== "string" || !raw.pending || typeof raw.pending !== "object") return null;
      const pending = Object.fromEntries(Object.entries(raw.pending).filter(([k, v]) => isUiSection(k) && v && typeof v === "object"));
      return { owner: raw.owner, pending };
    } catch {
      return null;
    }
  }

  function writeClaim(claim: Claim | null) {
    try {
      if (!claim || !Object.keys(claim.pending).length) storage?.removeItem(CLAIM_KEY);
      else storage?.setItem(CLAIM_KEY, JSON.stringify(claim));
    } catch {
      // Without storage the claim lives only as long as this page.
    }
  }

  async function load(gen: number, owner: string) {
    let doc: UiPreferencesDocument | undefined;
    try {
      doc = await api.getUiPreferences();
    } catch {
      doc = undefined;
    }
    if (gen !== generation) return;
    if (doc) for (const b of bindings) if (b.section in doc.sections && !dirty.has(b.section)) apply(b, doc.sections[b.section]);
    loaded = true;
    // A failed load keeps the owner's cache and never migrates.
    if (doc) await migrate(gen, owner, doc);
    if (gen !== generation) return;
    for (const b of bindings) if (dirty.has(b.section)) schedule(b);
  }

  async function migrate(gen: number, owner: string, doc: UiPreferencesDocument) {
    const claim = readClaim();
    if (!claim || claim.owner !== owner) return;
    for (const section of Object.keys(claim.pending) as UiSection[]) {
      if (gen !== generation) return;
      const value = claim.pending[section]!;
      if (!(section in doc.sections)) {
        try {
          await api.putUiPreference(section, value);
        } catch {
          return; // resumes with the next load of the same owner
        }
        if (gen !== generation) return;
        const binding = bindings.find((b) => b.section === section);
        if (binding && !dirty.has(section)) apply(binding, value);
      }
      const current = readClaim();
      if (!current || current.owner !== owner) return;
      delete current.pending[section];
      writeClaim(current);
    }
  }

  return {
    setIdentity(id) {
      if (id === identity) return;
      generation++;
      for (const t of timers.values()) clearTimeout(t);
      timers.clear();
      dirty.clear();
      loaded = false;
      identity = id;
      if (id === null) return;
      const owner = storage?.getItem(OWNER_KEY) ?? null;
      if (owner === null) {
        // Untagged values predate M17: claim them for this user before any upload.
        const pending: Claim["pending"] = {};
        for (const b of bindings) if (storage?.getItem(b.legacyKey) != null) pending[b.section] = b.toWire(b.store.get());
        if (!readClaim() && Object.keys(pending).length) writeClaim({ owner: id, pending });
      } else if (owner !== id) {
        resetStores();
      }
      try {
        storage?.setItem(OWNER_KEY, id);
      } catch {
        // Owner tagging is best effort; the server document stays authoritative.
      }
      void load(generation, id);
    },
    dispose() {
      generation++;
      for (const t of timers.values()) clearTimeout(t);
      unsubscribe.forEach((off) => off());
    },
  };
}
