import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import records from "@/test/fixtures/lab-preferences-v1.json";

// Captured with the pre-move stores at 3d449aa; no current serializer creates this fixture.
beforeEach(() => {
  vi.resetModules();
  localStorage.clear();
  for (const [key, record] of Object.entries(records)) localStorage.setItem(key, JSON.stringify(record));
});
afterEach(() => localStorage.clear());

describe("stored lab preferences", () => {
  it("loads palette, scheme, density, motion and comparison slots without rewriting storage", async () => {
    const { designStore } = await import("@/shared/theme/design-tokens");
    expect(designStore.get()).toEqual(records["docklet-react-opus.design-lab"].data);
    expect(JSON.parse(localStorage.getItem("docklet-react-opus.design-lab")!)).toEqual(records["docklet-react-opus.design-lab"]);
  });

  it("loads views, visual controls, explanations, hidden items and dashboard choices", async () => {
    const [{ preferencesStore }, { uiStyleStore }, { visualStyleStore }, { helpStore }, { hiddenStore }, { layoutStore }, { fleetViewStore }] = await Promise.all([
      import("./preferences-store"), import("./ui-style-store"), import("./visual-style-store"), import("./help-store"), import("./hidden-store"),
      import("@/features/dashboard/dashboard-layout"), import("@/features/dashboard/fleet-view-store"),
    ]);
    for (const [suffix, store] of [
      ["preferences", preferencesStore], ["ui-style", uiStyleStore], ["visual-style", visualStyleStore], ["help", helpStore],
      ["hidden", hiddenStore], ["dashboard", layoutStore], ["fleet-view", fleetViewStore],
    ] as const) {
      const stored = records[`docklet-react-opus.${suffix}`].data;
      // Older records acquire the new personal toggle without rewriting the fixture.
      expect(store.get()).toEqual(suffix === "preferences" ? { ...stored, showUnraidRing: false } : stored);
    }
  });

  it("restores and re-exports the pre-move favourite design code unchanged", async () => {
    const { favoriteStore, resetToFavorite, captureSnapshot, encodeDesignCode } = await import("./favorite-code");
    expect(favoriteStore.get()).toEqual(records["docklet-react-opus.design-favorite"].data);
    expect(resetToFavorite()).toBe("favorite");
    expect(encodeDesignCode(captureSnapshot())).toBe(records["docklet-react-opus.design-favorite"].data.code);
  });
});
