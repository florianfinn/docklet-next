// Source: docklet-next/issues/357
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original compile-time negative guards for contract catalogue totality.

import { expect, it } from "vitest";
import type { components } from "@/api/generated/schema";
import { MESSAGE_CODE_KEYS } from "./wire-catalogue-messages";
import { wireKey, type CatalogueKey } from "./wire-catalogue";

type ExtendedCode = NonNullable<components["schemas"]["ActivityMessage"]["code"]> | "example.new-contract-code";

// A future producer member must require a key, even when all existing members are covered.
// @ts-expect-error example.new-contract-code deliberately has no catalogue mapping.
const incomplete: Record<ExtendedCode, CatalogueKey> = MESSAGE_CODE_KEYS;
// @ts-expect-error A string that is not a catalogue leaf must not satisfy CatalogueKey.
const invalidKey: CatalogueKey = "example.missing-catalogue-key";

it("keeps compile-time negative probes and runtime unknown-code lookup separate", () => {
  expect(incomplete).toBe(MESSAGE_CODE_KEYS);
  expect(wireKey(MESSAGE_CODE_KEYS, invalidKey)).toBeUndefined();
  expect(wireKey(MESSAGE_CODE_KEYS, "toString")).toBeUndefined();
});
