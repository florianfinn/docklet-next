// Source: docklet-next/issues/350
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original image detail metadata mapping. M19 web consumer integration.

import type { DockletAdapter } from "../adapter";
import { AdapterOutcomeError } from "../capability";
import { splitInventoryId } from "./inventory-mapping";
import { imageResponseError } from "./image-errors";

export function createImageMetadata(baseUrl: string, fetcher: typeof fetch): Pick<DockletAdapter, "getImageMetadata"> {
  return { async getImageMetadata(id) {
    const [host, raw] = splitInventoryId(id);
    const response = await fetcher(`${baseUrl}/environments/${encodeURIComponent(host)}/images/${encodeURIComponent(raw)}`, { headers: { Accept: "application/json" } });
    const body = await response.json();
    if (!response.ok) throw imageResponseError(body, response.status, false);
    const row = body?.data;
    if (body?.success !== true || typeof row?.architecture !== "string" || typeof row.os !== "string" || row.rootFs?.layers !== null && (!Array.isArray(row.rootFs?.layers) || row.rootFs.layers.some((v: unknown) => typeof v !== "string"))) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    return { architecture: row.architecture, os: row.os, layers: row.rootFs.layers?.length ?? 0 };
  } };
}
