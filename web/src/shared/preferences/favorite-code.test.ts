import { afterEach, describe, expect, it } from "vitest";

import { DEFAULT_LAYOUT, layoutStore, setService } from "@/features/dashboard/dashboard-layout";
import { DEFAULT_FLEET_VIEW, fleetViewStore } from "@/features/dashboard/fleet-view-store";
import { helpStore } from "@/shared/preferences/help-store";
import { DEFAULT_PREFERENCES, preferencesStore } from "@/shared/preferences/preferences-store";
import { DEFAULT_UI_STYLE, uiStyleStore } from "@/shared/preferences/ui-style-store";
import { DEFAULT_VISUAL_STYLE, visualStyleStore } from "@/shared/preferences/visual-style-store";

import { DEFAULT_TOKENS, designStore, normalizeTokens, TOKEN_RANGES } from "@/shared/theme/design-tokens";
import { applySnapshot, captureSnapshot, clearFavorite, decodeDesignCode, defaultSnapshot, encodeDesignCode, favoriteStore, mergeFavorite, resetToFavorite, resetToShipped, saveFavorite, SHIPPED_DEFAULT_CODE } from "@/shared/preferences/favorite-code";

afterEach(() => {
  for (const store of [layoutStore, fleetViewStore, preferencesStore, uiStyleStore, visualStyleStore, designStore, favoriteStore, helpStore]) store.reset();
  localStorage.clear();
});

const b64 = (value: unknown) => btoa(JSON.stringify(value)).replace(/\+/g, "-").replace(/\//g, "_").replace(/=+$/, "");

describe("design code", () => {
  it("round-trips the presentation settings and never carries hosts, addresses or secrets", () => {
    visualStyleStore.set({ ...DEFAULT_VISUAL_STYLE, glowInside: 40, logoVariant: "initial" });
    layoutStore.set((l) => setService({ ...l, hidden: ["apps"], sizes: { load: { w: 2, h: 5 } } }, "plex", { address: "http://plex.example.test:32400", status: "connected" }));
    fleetViewStore.set((f) => ({ ...f, mode: "perHost", hostOrder: ["delta", "atlas"] }));
    preferencesStore.set((p) => ({ ...p, hostSelection: ["atlas"], stacksLayout: "hosts" }));
    helpStore.set({ mode: "compact" });
    const code = encodeDesignCode(captureSnapshot());
    const text = atob(code.split(":")[1]!.replace(/-/g, "+").replace(/_/g, "/"));
    expect(["plex.example", "32400", "atlas", "delta", "hostSelection", "hostOrder", "services", "ignoreReducedMotion"].filter((s) => text.includes(s))).toEqual([]);
    const result = decodeDesignCode(code);
    expect(result.ok && result.sections).toEqual(["tokens", "uiStyle", "visual", "dashboard", "fleet", "overview", "help"]);
    if (!result.ok) return;
    expect([result.snapshot.visual?.glowInside, result.snapshot.visual?.logoVariant, result.snapshot.dashboard?.sizes.load, result.snapshot.fleet?.mode, result.snapshot.overview?.stacksLayout, result.snapshot.help?.mode]).toEqual([40, "initial", { w: 2, h: 5 }, "perHost", "hosts", "compact"]);
    // A code without the explanation mode (earlier favourites) leaves it as it is.
    helpStore.set({ mode: "off" });
    applySnapshot({ overview: result.snapshot.overview });
    expect(helpStore.get().mode).toBe("off");
  });

  it("rejects foreign, damaged, oversized and future codes with a specific error", () => {
    const err = (code: string) => {
      const r = decodeDesignCode(code);
      return r.ok ? "ok" : r.error;
    };
    expect([err(""), err("hello"), err("DOCKLET-DESIGN-1:%%%"), err("DOCKLET-DESIGN-1:abc"), err("DOCKLET-DESIGN-2:abc"), err(`DOCKLET-DESIGN-1:${"a".repeat(30_000)}`)]).toEqual(["empty", "prefix", "decode", "decode", "version", "size"]);
    expect(err(`DOCKLET-DESIGN-1:${b64({ schema: "other", sections: {} })}`)).toBe("schema");
    expect(err(`DOCKLET-DESIGN-1:${b64({ schema: "docklet-design", sections: { unknown: { a: 1 } } })}`)).toBe("schema");
  });

  it("validates every field of a hand-made code and drops unknown keys and address-like strings", () => {
    const code = `DOCKLET-DESIGN-1:${b64({
      schema: "docklet-design",
      sections: {
        visual: { glowInside: 999, logoMotion: "spin", dotWave: "yes", token: "secret" },
        uiStyle: { toggleStyle: "https://evil.example.test/x", pointerGlowIntensity: 7, apiKey: "abc" },
        dashboard: { order: ["load", "nope"], hidden: [], sizes: { load: { w: 99, h: 99 } }, services: { plex: { address: "http://x" } } },
        fleet: { mode: "line", ringSize: 1000, hostOrder: ["atlas"] },
      },
    })}`;
    const r = decodeDesignCode(code);
    expect(r.ok).toBe(true);
    if (!r.ok) return;
    expect([r.snapshot.visual?.glowInside, r.snapshot.visual?.logoMotion, r.snapshot.visual?.dotWave, "token" in (r.snapshot.visual ?? {})]).toEqual([64, "relay", true, false]);
    expect([r.snapshot.uiStyle?.toggleStyle, r.snapshot.uiStyle?.pointerGlowIntensity, "apiKey" in (r.snapshot.uiStyle ?? {})]).toEqual([DEFAULT_UI_STYLE.toggleStyle, 1, false]);
    expect([r.snapshot.dashboard?.order[0], r.snapshot.dashboard?.order.includes("nope" as never), r.snapshot.dashboard?.sizes.load, "services" in (r.snapshot.dashboard ?? {})]).toEqual(["load", false, { w: 4, h: 10 }, false]);
    expect([r.snapshot.fleet?.ringSize, "hostOrder" in (r.snapshot.fleet ?? {})]).toEqual([320, false]);
  });

  it("applies through the stores and keeps host order, host selection and widget connections", () => {
    layoutStore.set((l) => setService(l, "plex", { address: "http://plex.example.test", status: "connected" }));
    fleetViewStore.set((f) => ({ ...f, hostOrder: ["cedar"] }));
    preferencesStore.set((p) => ({ ...p, hostSelection: ["atlas"] }));
    const before = designStore.get().tokens.ignoreReducedMotion;
    const r = decodeDesignCode(`DOCKLET-DESIGN-1:${b64({ schema: "docklet-design", sections: { visual: { dotWave: false }, dashboard: { order: ["security"], hidden: ["load"], sizes: {} }, fleet: { mode: "line" }, overview: { containersMode: "advanced" } } })}`);
    if (!r.ok) throw new Error("decode");
    expect(applySnapshot(r.snapshot)).toEqual(["visual", "dashboard", "fleet", "overview"]);
    expect([visualStyleStore.get().dotWave, layoutStore.get().order[0], layoutStore.get().hidden.includes("load"), layoutStore.get().services.plex.address, fleetViewStore.get().mode, fleetViewStore.get().hostOrder, preferencesStore.get().hostSelection, preferencesStore.get().containersMode, designStore.get().tokens.ignoreReducedMotion]).toEqual([
      false,
      "security",
      true,
      "http://plex.example.test",
      "line",
      ["cedar"],
      ["atlas"],
      "advanced",
      before,
    ]);
  });

  it("saves a favourite, resets to it, and falls back to the shipped default without one", () => {
    visualStyleStore.set({ ...DEFAULT_VISUAL_STYLE, glowOutside: 30 });
    saveFavorite(new Date("2026-10-11T10:00:00Z"));
    visualStyleStore.set({ ...DEFAULT_VISUAL_STYLE, glowOutside: 10 });
    expect([resetToFavorite(), visualStyleStore.get().glowOutside]).toEqual(["favorite", 30]);
    clearFavorite();
    expect([resetToFavorite(), visualStyleStore.get().glowOutside]).toEqual(["shipped", DEFAULT_VISUAL_STYLE.glowOutside]);
    // A tampered favourite in storage is dropped on load instead of being applied.
    expect([mergeFavorite({ code: "DOCKLET-DESIGN-1:broken", savedAt: "2026-10-11" }), mergeFavorite({ code: 3 }), mergeFavorite(saveFavorite(new Date("2026-10-11T10:00:00Z")))?.savedAt]).toEqual([null, null, "2026-10-11T10:00:00.000Z"]);
  });
  it("rejects unknown enums and clamps numeric tokens and UI style to their ranges", () => {
    const r = decodeDesignCode(`DOCKLET-DESIGN-1:${b64({
      schema: "docklet-design",
      sections: {
        tokens: { theme: "neon", motion: "wild", density: "huge", semantic: 9, glow: -4, speed: 0, radius: 40, fontScale: 3, palette: { primary: "red;x", secondary: "#a4b4ff" } },
        uiStyle: { toggleStyle: "chrome", metricDisplay: "pie", hostFilterStyle: "tree", toastScale: 0.1, toastDesign: "huge", actionDialogDesign: "modal", actionPresentationOverride: "always" },
      },
    })}`);
    if (!r.ok) throw new Error("decode");
    const t = r.snapshot.tokens;
    expect([t?.theme, t?.motion, t?.density]).toEqual([DEFAULT_TOKENS.theme, DEFAULT_TOKENS.motion, DEFAULT_TOKENS.density]);
    expect([t?.semantic, t?.glow, t?.speed, t?.radius, t?.fontScale]).toEqual([TOKEN_RANGES.semantic.max, TOKEN_RANGES.glow.min, TOKEN_RANGES.speed.min, TOKEN_RANGES.radius.max, TOKEN_RANGES.fontScale.max]);
    expect([t?.palette.primary, t?.palette.secondary]).toEqual([DEFAULT_TOKENS.palette.primary, "#a4b4ff"]);
    const u = r.snapshot.uiStyle;
    expect([u?.toggleStyle, u?.metricDisplay, u?.hostFilterStyle, u?.toastScale, u?.toastDesign, u?.actionDialogDesign, u?.actionPresentationOverride]).toEqual([DEFAULT_UI_STYLE.toggleStyle, DEFAULT_UI_STYLE.metricDisplay, DEFAULT_UI_STYLE.hostFilterStyle, 0.9, "bar", "focus", "hub"]);
    // Stored lab state passes the same checks on reload.
    const stored = normalizeTokens({ ...DEFAULT_TOKENS, motion: "wild", speed: Number.NaN, ignoreReducedMotion: "yes" });
    expect([stored.motion, stored.speed, stored.ignoreReducedMotion]).toEqual([DEFAULT_TOKENS.motion, DEFAULT_TOKENS.speed, false]);
  });

  it("carries the notification and action dialog choices through a code", () => {
    uiStyleStore.set({ ...DEFAULT_UI_STYLE, toastDesign: "edge", actionDialogDesign: "console", actionPresentationOverride: "toast", toastScale: 1.5 });
    const code = encodeDesignCode(captureSnapshot());
    uiStyleStore.reset();
    const r = decodeDesignCode(code);
    if (!r.ok) throw new Error("decode");
    applySnapshot(r.snapshot);
    const u = uiStyleStore.get();
    expect([u.toastDesign, u.actionDialogDesign, u.actionPresentationOverride, u.toastScale]).toEqual(["edge", "console", "toast", 1.5]);
  });
});

/** The user's favourite DOCKLET-DESIGN-1 code from the designer trial, decoded; the shipped default. */
const USER_FAVORITE = {
  schema: "docklet-design",
  sections: {
    tokens: { theme: "dark", palette: { primary: "#9333ea", secondary: "#00ffcc", surface: "#13111c" }, semantic: 1.12, glow: 1, motion: "playful", speed: 1, density: "compact", radius: 0.8, fontScale: 1 },
    uiStyle: { toggleStyle: "minimal", metricDisplay: "bars", hostFilterStyle: "menu", pointerGlow: true, pointerGlowIntensity: 0.6, toastScale: 1.6, toastDesign: "bar", actionDialogDesign: "focus", actionPresentationOverride: "hub", runMotion: "sweep" },
    visual: { logoVariant: "dropcap", logoMotion: "relay", logoEntrance: true, glowInside: 50, glowOutside: 4, glowSize: 128, glowLine: 2, dotWave: true, dotWaveStrength: 0.9, dotWavePeriod: 32, dotDrift: 0, sidebarWidth: 17.5 },
    dashboard: {
      order: ["attention", "updates", "load", "security", "capacity", "events", "timeline", "overseerr", "plex", "schedule", "minecraft", "apps"],
      hidden: ["minecraft", "load", "timeline", "capacity", "updates"],
      sizes: { attention: { w: 1, h: 6 }, updates: { w: 1, h: 6 }, timeline: { w: 2, h: 4 }, security: { w: 2, h: 3 }, load: { w: 2, h: 3 }, apps: { w: 2, h: 3 }, overseerr: { w: 1, h: 6 }, events: { w: 1, h: 6 }, schedule: { w: 1, h: 6 }, plex: { w: 1, h: 6 } },
    },
    fleet: { mode: "joint", ringSize: 320, placement: "right", fan: true, hostArea: { w: 5, h: 2 } },
    overview: { stacksMode: "compact", containersMode: "compact", stacksLayout: "hosts", containersLayout: "hosts", volumesLayout: "table", imagesLayout: "table", workspaceAside: "details" },
  },
};

describe("shipped default design", () => {
  it("is the user's favourite code in every store on a first visit", () => {
    const { dotDrift: _orbit, ...visual } = USER_FAVORITE.sections.visual;
    const shipped = defaultSnapshot();
    expect([shipped.tokens, shipped.uiStyle, shipped.dashboard, shipped.fleet, shipped.overview]).toEqual([USER_FAVORITE.sections.tokens, USER_FAVORITE.sections.uiStyle, USER_FAVORITE.sections.dashboard, USER_FAVORITE.sections.fleet, USER_FAVORITE.sections.overview]);
    // Every visual field of the code, plus the new 3D depth on its default; the old orbit key is gone.
    expect([{ ...shipped.visual, dotDepth: undefined }, (shipped.visual?.dotDepth ?? 0) > 0]).toEqual([{ ...visual, dotDepth: undefined }, true]);
    // Fresh stores start from the same data, independently of each other.
    expect([designStore.get().tokens.palette, uiStyleStore.get().runMotion, layoutStore.get().hidden, fleetViewStore.get().placement, fleetViewStore.get().hostArea, preferencesStore.get().stacksLayout, visualStyleStore.get().logoMotion]).toEqual([USER_FAVORITE.sections.tokens.palette, "sweep", USER_FAVORITE.sections.dashboard.hidden, "right", { w: 5, h: 2 }, "hosts", "relay"]);
    expect([DEFAULT_LAYOUT.services.plex.status, DEFAULT_FLEET_VIEW.hostOrder, DEFAULT_PREFERENCES.hostSelection]).toEqual(["unconfigured", [], []]);
  });

  it("decodes the user's code, applies it, round-trips it and resets back to it", () => {
    const code = `DOCKLET-DESIGN-1:${b64(USER_FAVORITE)}`;
    const r = decodeDesignCode(code);
    expect(r.ok && r.sections).toEqual(["tokens", "uiStyle", "visual", "dashboard", "fleet", "overview"]);
    // A browser that changed things keeps them until it asks for the default.
    fleetViewStore.set((f) => ({ ...f, placement: "below", hostOrder: ["delta"] }));
    preferencesStore.set((p) => ({ ...p, stacksLayout: "table", hostSelection: ["atlas"] }));
    visualStyleStore.set({ ...DEFAULT_VISUAL_STYLE, dotWave: false });
    expect([fleetViewStore.get().placement, preferencesStore.get().stacksLayout]).toEqual(["below", "table"]);
    expect(resetToShipped()).toBe("shipped");
    expect([fleetViewStore.get().placement, fleetViewStore.get().hostOrder, preferencesStore.get().stacksLayout, preferencesStore.get().hostSelection, visualStyleStore.get().dotWave]).toEqual(["right", ["delta"], "hosts", ["atlas"], true]);
    // Applying the user's code gives the same state as the shipped default, and exporting it gives the shipped code.
    if (!r.ok) throw new Error("decode");
    applySnapshot(r.snapshot);
    expect(encodeDesignCode(captureSnapshot())).toBe(SHIPPED_DEFAULT_CODE);
  });

  it("keeps the rest of a code when one enum is unknown", () => {
    const r = decodeDesignCode(`DOCKLET-DESIGN-1:${b64({ ...USER_FAVORITE, sections: { ...USER_FAVORITE.sections, uiStyle: { ...USER_FAVORITE.sections.uiStyle, runMotion: "glide" } } })}`);
    expect(r.ok && [r.snapshot.uiStyle?.runMotion, r.snapshot.uiStyle?.toastScale, r.snapshot.fleet?.ringSize]).toEqual(["sweep", 1.6, 320]);
  });
});

