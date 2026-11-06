// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original one-time import consumer with identity and draft ownership. M19 web consumer integration.

import { CancelledError } from "@tanstack/react-query";
import type { DockletAdapter, ImportResult } from "../adapter";
import { AdapterOutcomeError, HUB_REF, unavailable } from "../capability";
import type { CsrfClient } from "./csrf-client";
import { IMPORT_FOUND_KEYS, IMPORT_MISSING_KEYS } from "../wire-catalogue-import";

export function createStackImport(baseUrl: string, client: CsrfClient, generation: () => number, capability: DockletAdapter["getCapability"]): Pick<DockletAdapter, "importSource"> {
  return { async importSource(input) {
    const owner = generation();
    const check = () => { if (generation() !== owner) throw new CancelledError(); };
    const grant = await capability("importSource", HUB_REF); check();
    if (!grant.available) throw unavailable("importSource", HUB_REF, grant.disabledReasonCode);
    const response = await client.fetch(`${baseUrl}/stack-import`, { method: "POST", headers: { Accept: "application/json", "Content-Type": "application/json" }, body: JSON.stringify({ ...input, kind: input.kind === "dockerhub" ? "registry" : "github" }) });
    check();
    const body = await response.json(); check();
    if (!response.ok) throw new AdapterOutcomeError({ kind: "error", code: typeof body?.code === "string" ? body.code : "http.error", status: response.status });
    const data = body;
    if (typeof data?.title !== "string" || typeof data.composeContent !== "string" || !Array.isArray(data.envNames) || data.envNames.some((v: unknown) => typeof v !== "string" || !/^[A-Za-z_][A-Za-z0-9_]*$/.test(v)) || !Array.isArray(data.found) || data.found.some((v: string) => !Object.hasOwn(IMPORT_FOUND_KEYS, v)) || !Array.isArray(data.missing) || data.missing.some((v: string) => !Object.hasOwn(IMPORT_MISSING_KEYS, v))) throw new AdapterOutcomeError({ kind: "error", code: "http.error" });
    return { title: data.title, composeContent: data.composeContent, envNames: data.envNames, found: data.found, missing: data.missing } satisfies ImportResult;
  } };
}
