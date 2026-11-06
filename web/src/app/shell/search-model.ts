// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role-scoped command palette using existing query consumers. M19 web consumer integration.

import { defaultFilter } from "cmdk";
export interface SearchEntry { id: string; label: string; host?: string; keywords?: string[]; to?: string; run?: () => void }
export function matchSearch(entries: SearchEntry[], query: string) {
  return entries.filter((item) => !query.trim() || defaultFilter([item.label, item.host, ...(item.keywords ?? [])].filter(Boolean).join(" "), query.trim()) > 0).slice(0, 8);
}
export function isSearchShortcut(event: KeyboardEvent) {
  return (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k" && !(event.target instanceof Element && event.target.closest('[data-slot="code-editor"]'));
}
