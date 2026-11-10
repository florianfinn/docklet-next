// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original shared plain-text advisory content and exact occurrence actions. M21 #437: advisory detail.

import type { AdvisoryOccurrence } from "@/data/vulnerability-advisory";
import { decodeAdvisoryKey } from "@/data/vulnerability-advisory";

export interface OccurrenceSelection { host: string; imageId: string; package: string; installed: string; class: string; type: string }
export function selectionFor(host: string, o: AdvisoryOccurrence): OccurrenceSelection {
  return { host, imageId: o.imageId, package: o.pkgName, installed: o.installedVersion, class: o.class ?? "", type: o.type ?? "" };
}
export function occurrenceSelectionKey(s: OccurrenceSelection): string {
  return btoa(Array.from(new TextEncoder().encode(JSON.stringify(s)), (v) => String.fromCharCode(v)).join("")).replaceAll("+", "-").replaceAll("/", "_").replace(/=+$/, "");
}

export function advisoryLabel(key: string) { try { return decodeAdvisoryKey(key); } catch { return ""; } }
