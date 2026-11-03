// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original break-map tests PF1–PF4 and PF9 of the profile preference sync for issue #339.

import { act, screen } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import { createMemoryUiPreferences } from "@/data/demo/demo-ui-preferences";
import type { UiPreferencesApi, UiPreferencesDocument, UiSection } from "@/domain/ui-preferences";
import DashboardPage from "@/features/dashboard/DashboardPage";
import { DEFAULT_LAYOUT, layoutStore } from "@/features/dashboard/dashboard-layout";
import { clearFavorite, favoriteStore, saveFavorite } from "@/shared/preferences/favorite-code";
import { DEFAULT_HELP, helpStore, setHelpMode } from "@/shared/preferences/help-store";
import { hiddenStore } from "@/shared/preferences/hidden-store";
import { DEFAULT_PREFERENCES, preferencesStore } from "@/shared/preferences/preferences-store";
import { renderWithApp } from "@/test/render";

import { CLAIM_KEY, createPreferenceSync, OWNER_KEY, type PreferenceSync } from "./engine";
import { SECTION_BINDINGS } from "./sections";

type Server = ReturnType<typeof createMemoryUiPreferences>;

/** Routes each request to the document of the identity that sent it, like a session cookie. */
function hub() {
  const servers = new Map<string, Server>();
  const puts: [string, UiSection, unknown][] = [];
  let session = "";
  const gates: { load?: Promise<void>; failPut?: (section: UiSection) => boolean; failLoad?: boolean } = {};
  const server = (id: string) => servers.get(id) ?? servers.set(id, createMemoryUiPreferences()).get(id)!;
  const api: UiPreferencesApi = {
    async getUiPreferences() {
      const as = session;
      if (gates.failLoad) throw new Error("load failed");
      if (gates.load) await gates.load;
      return server(as).getUiPreferences();
    },
    async putUiPreference(section, value) {
      const as = session;
      if (gates.failPut?.(section)) throw new Error("put failed");
      puts.push([as, section, structuredClone(value)]);
      return server(as).putUiPreference(section, value);
    },
  };
  return { api, puts, gates, server, signIn: (id: string) => { session = id; } };
}

function legacy(key: string, data: unknown) {
  localStorage.setItem(key, JSON.stringify({ v: 1, data }));
}

const flush = async () => { for (let i = 0; i < 6; i++) await Promise.resolve(); };
let sync: PreferenceSync | undefined;

function start(api: UiPreferencesApi) {
  sync = createPreferenceSync({ api, bindings: SECTION_BINDINGS, storage: localStorage, debounceMs: 1000 });
  return sync;
}

beforeEach(() => {
  vi.useFakeTimers();
  localStorage.clear();
  for (const b of SECTION_BINDINGS) b.store.set(b.fromWire(undefined, b.store.get()));
  localStorage.clear();
});

afterEach(() => {
  sync?.dispose();
  sync = undefined;
  vi.useRealTimers();
});

describe("identity changes and late responses (PF1)", () => {
  it("drops a debounced edit when the user signs out and never sends it for the next user", async () => {
    const h = hub();
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    setHelpMode("off");
    s.setIdentity(null);
    h.signIn("b");
    s.setIdentity("b");
    await flush();
    await vi.advanceTimersByTimeAsync(5000);
    expect(h.puts).toEqual([]);
    expect(helpStore.get()).toEqual(DEFAULT_HELP);
  });

  it("ignores a load answer of the previous user and does not echo hydration", async () => {
    const h = hub();
    await h.server("a").putUiPreference("help", { mode: "off" });
    await h.server("b").putUiPreference("help", { mode: "compact" });
    let release!: () => void;
    h.gates.load = new Promise((r) => { release = r; });
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    h.gates.load = undefined;
    h.signIn("b");
    s.setIdentity("b");
    await flush();
    release();
    await flush();
    await vi.advanceTimersByTimeAsync(5000);
    expect([helpStore.get().mode, h.puts]).toEqual(["compact", []]);
  });
});

describe("owner-bound legacy import (PF2)", () => {
  it("claims pre-M17 values for the first user, resumes only for that user and never overwrites the hub", async () => {
    legacy("docklet-react-opus.help", { mode: "off" });
    legacy("docklet-react-opus.hidden", { stacks: ["h~legacy"], containers: [] });
    legacy("docklet-react-opus.preferences", { ...DEFAULT_PREFERENCES, stacksMode: "advanced" });
    helpStore.set({ mode: "off" });
    hiddenStore.set({ stacks: ["h~legacy"], containers: [] });
    preferencesStore.set({ ...DEFAULT_PREFERENCES, stacksMode: "advanced" });
    const h = hub();
    await h.server("a").putUiPreference("views", { ...DEFAULT_PREFERENCES, stacksMode: "compact" });
    h.gates.failPut = (section) => section === "help";
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    // Interrupted after the first upload: hidden is on the hub, help is still claimed.
    expect(h.puts.map(([as, section]) => [as, section])).toEqual([["a", "hidden"]]);
    expect(JSON.parse(localStorage.getItem(CLAIM_KEY)!).owner).toBe("a");
    expect(h.server("a").document().sections.views).toMatchObject({ stacksMode: "compact" });

    h.gates.failPut = undefined;
    s.setIdentity(null);
    h.signIn("b");
    s.setIdentity("b");
    await flush();
    await vi.advanceTimersByTimeAsync(5000);
    expect(h.puts.filter(([as]) => as === "b")).toEqual([]);
    expect([hiddenStore.get().stacks, helpStore.get().mode]).toEqual([[], DEFAULT_HELP.mode]);

    s.setIdentity(null);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    expect(h.puts.map(([as, section]) => [as, section])).toEqual([["a", "hidden"], ["a", "help"]]);
    expect([hiddenStore.get().stacks, helpStore.get().mode]).toEqual([["h~legacy"], "off"]);
    expect(localStorage.getItem(CLAIM_KEY)).toBeNull();
    expect(localStorage.getItem(OWNER_KEY)).toBe("a");
  });
});

describe("failed loads and edits during the load (PF3)", () => {
  it("keeps the owner cache and does not migrate when the load fails", async () => {
    legacy("docklet-react-opus.help", { mode: "compact" });
    helpStore.set({ mode: "compact" });
    const h = hub();
    h.gates.failLoad = true;
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    await vi.advanceTimersByTimeAsync(5000);
    expect([helpStore.get().mode, h.puts, JSON.parse(localStorage.getItem(CLAIM_KEY)!).owner]).toEqual(["compact", [], "a"]);
  });

  it("keeps an edit made while the load is pending and sends it afterwards", async () => {
    const h = hub();
    await h.server("a").putUiPreference("help", { mode: "compact" });
    let release!: () => void;
    h.gates.load = new Promise((r) => { release = r; });
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    setHelpMode("off");
    release();
    await flush();
    expect(helpStore.get().mode).toBe("off");
    await vi.advanceTimersByTimeAsync(1000);
    expect(h.server("a").document().sections.help).toEqual({ mode: "off" });
  });
});

describe("decoders at the sync boundary (PF4)", () => {
  it("renders real consumers from a nested malformed document", async () => {
    const h = hub();
    const doc: UiPreferencesDocument = { revision: 3, sections: { views: { hostSelection: null, stacksMode: 7 }, dashboard: { order: {}, hidden: "x", sizes: [] }, help: { mode: ["full"] }, hidden: { stacks: "h~x" } } };
    h.api.getUiPreferences = async () => doc;
    const s = start(h.api);
    s.setIdentity("a");
    await flush();
    expect([preferencesStore.get().hostSelection, preferencesStore.get().stacksMode, layoutStore.get().order, helpStore.get().mode, hiddenStore.get().stacks]).toEqual([[], DEFAULT_PREFERENCES.stacksMode, DEFAULT_LAYOUT.order, DEFAULT_HELP.mode, []]);
    vi.useRealTimers();
    renderWithApp(<DashboardPage />);
    expect(await screen.findByRole("heading", { level: 1 })).toBeTruthy();
  });
});

describe("favourite clear representation (PF9)", () => {
  it("sends the cleared favourite as an envelope so a fresh browser gets none", async () => {
    const h = hub();
    const s = start(h.api);
    h.signIn("a");
    s.setIdentity("a");
    await flush();
    act(() => { saveFavorite(new Date("2026-11-03T10:00:00Z")); });
    await vi.advanceTimersByTimeAsync(1000);
    expect((h.server("a").document().sections.favorite as { value: unknown }).value).not.toBeNull();
    act(() => clearFavorite());
    await vi.advanceTimersByTimeAsync(1000);
    expect(h.server("a").document().sections.favorite).toEqual({ value: null });
    s.dispose();
    localStorage.clear();
    saveFavorite(new Date("2026-11-03T10:00:00Z"));
    start(h.api).setIdentity("a");
    await flush();
    expect(favoriteStore.get()).toBeNull();
  });
});
