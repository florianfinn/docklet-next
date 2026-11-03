// Source: docklet-next/docs/design/runtime-history.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original runtime history transport for issues #335, #336 and #337.

import type { HistoryApi } from "@/domain/history";
import type { UiPreferencesApi } from "@/domain/ui-preferences";
import { unavailable } from "../capability";

/** Transport is wired to the hub history routes once their contract lands; until then the sections show unavailable. */
export function createHttpHistory(): HistoryApi {
  const missing = async (): Promise<never> => { throw unavailable("get-runtime-history"); };
  return { getContainerHistory: missing, getStackHistory: missing, getCapacity: missing };
}

/** Profile preference transport; wired to the hub routes together with the history contract. */
export function createHttpUiPreferences(): UiPreferencesApi {
  const missing = async (): Promise<never> => { throw unavailable("ui-preferences"); };
  return { getUiPreferences: missing, putUiPreference: missing };
}
