// Source: docklet-next/issues/356
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original role-scoped command palette using existing query consumers. M19 web consumer integration; rank label matches before truncation.

import { defaultFilter } from "cmdk";
export interface SearchEntry { id: string; label: string; host?: string; keywords?: string[]; to?: string; run?: () => void }
export function matchSearch(entries: SearchEntry[], query: string) {
  const needle = query.trim().toLowerCase();
  if (!needle) return entries.slice(0, 8);
  return entries.map(item => {
    const label = item.label.toLowerCase();
    const fuzzy = defaultFilter([item.label, item.host, ...(item.keywords ?? [])].filter(Boolean).join(" "), needle);
    return { item, rank: label === needle ? 2 : label.startsWith(needle) ? 1 : 0, fuzzy };
  }).filter(match => match.fuzzy > 0)
    .sort((a, b) => b.rank - a.rank)
    .slice(0, 8).map(match => match.item);
}
export function isSearchShortcut(event: KeyboardEvent) {
  return (event.ctrlKey || event.metaKey) && event.key.toLowerCase() === "k" && !(event.target instanceof Element && event.target.closest('[data-slot="code-editor"]'));
}
