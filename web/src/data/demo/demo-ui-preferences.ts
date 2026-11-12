// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original in-memory preference document with the hub's limits for issue #339.

import { isUiSection, UI_DOCUMENT_LIMIT, UI_SECTION_LIMIT, type UiPreferencesApi, type UiPreferencesDocument } from "@/domain/ui-preferences";

import { AdapterOutcomeError } from "../capability";

const reject = (code: string, status: number) => new AdapterOutcomeError({ kind: "error", code, status });
const size = (value: unknown) => new TextEncoder().encode(JSON.stringify(value)).length;

/** Same allowlist, object check and size limits as the hub route; responses are copies. */
export function createMemoryUiPreferences(initial: UiPreferencesDocument = { revision: 0, sections: {} }): UiPreferencesApi & { document(): UiPreferencesDocument } {
  let doc: UiPreferencesDocument = structuredClone(initial);
  return {
    document: () => structuredClone(doc),
    getUiPreferences: async () => structuredClone(doc),
    async putUiPreference(section, value) {
      if (!isUiSection(section)) throw reject("preferences.unknown-section", 400);
      if (!value || typeof value !== "object" || Array.isArray(value)) throw reject("preferences.invalid", 400);
      const next = { revision: doc.revision + 1, sections: { ...doc.sections, [section]: structuredClone(value) } };
      if (size(value) > UI_SECTION_LIMIT || size(next) > UI_DOCUMENT_LIMIT) throw reject("preferences.too-large", 413);
      doc = next;
      return structuredClone(doc);
    },
  };
}
