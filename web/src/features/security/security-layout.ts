// Source: docklet-next/issues/266
// Revision: 5f56262875af9a8ec0fe9007cb889bd3a513085d
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original column group policy for measured security tables.

import type { FitView } from "@/shared/lib/fit-view";

export const FINDING_GROUPS = [["id", "severity"], ["pkg", "installed", "fixed"], ["image", "host"], ["cvss", "risk", "epss"]] as const;
export const IMAGE_GROUPS = [["image", "stack"], ["host", "usage"], ["critical", "high", "medium", "low", "unknown"], ["scanned"]] as const;

/** Cards and full tables retain each column; condensed groups retain each field with its label. */
export function securityColumnGroups<K extends string>(view: FitView, columns: readonly K[], condensed: readonly (readonly K[])[]): readonly (readonly K[])[] {
  return view === "condensed" ? condensed : columns.map((id) => [id]);
}
