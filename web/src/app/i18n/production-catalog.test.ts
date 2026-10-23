// Source: docklet-next/docs/design/adapter-live-capabilities.md
// Revision: 3d2d4f27b524b3422d010d6a69a22fe57225a78f
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original production boundary and regression coverage for issue #27.

import { afterAll, beforeAll, expect, it, vi } from "vitest";

import { leaves, validateCatalog } from "./catalog-validation.test-support";
import { productionEmptyExceptions } from "./production-empty-exceptions.test-support";

let catalogs: typeof import("./catalog").catalogs;
beforeAll(async () => {
  vi.stubEnv("DEV", false);
  vi.resetModules();
  ({ catalogs } = await import("./catalog"));
});
afterAll(() => vi.unstubAllEnvs());

it.each(["de", "en-US"] as const)("has no unreviewed empty %s production catalogue values", (locale) => {
  expect(import.meta.env.DEV).toBe(false);
  const empty = leaves(catalogs[locale]).filter(([, value]) => !value.trim()).map(([key]) => key).sort();
  expect(empty.filter((key) => !productionEmptyExceptions[key])).toEqual([]);
  expect(empty).toEqual(Object.keys(productionEmptyExceptions).sort());
  expect(Object.values(productionEmptyExceptions).every((reason) => reason.trim().length > 20)).toBe(true);
  expect(catalogs[locale].preview).toEqual({});
  expect(catalogs[locale].lab).toEqual({});
});

it("keeps production ICU arguments and translations aligned", () => {
  expect(validateCatalog(catalogs["en-US"], catalogs.de).filter((error) => !error.endsWith(": apostrophe") && !Object.keys(productionEmptyExceptions).some((key) => error === `base.${key}: empty message` || error === `translation.${key}: empty message`))).toEqual([]);
});
