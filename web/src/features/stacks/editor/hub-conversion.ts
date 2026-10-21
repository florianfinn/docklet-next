// Source: docklet-next/docs/design/stack-authoring.md
// Revision: e1fda6eb0c13b6b5cd95a59f65836bf68339de61
// Retrieved: 2026-10-21
// License: Apache-2.0
// Deviations: Original hub docker run result mapping for issue #139.

import { parse } from "yaml";

import type { Conversion } from "@/domain/compose/docker-run";
import { uniqueKey } from "@/domain/compose/ops";

export type Declarations = Partial<Record<"networks" | "volumes", Record<string, unknown>>>;

export type HubResult = { ok: true; conv: Conversion; env: string; declarations: Declarations } | { ok: false; reason: "invalid" | "failed" };

/** Takes the converted service out of the hub's Compose document. */
export function hubConversion(compose: string, env: string, serviceName: string, existing: string[]): HubResult {
  const doc = parse(compose) as { services?: Record<string, unknown>; networks?: unknown; volumes?: unknown } | null;
  const services = doc?.services;
  const body = services?.[serviceName];
  if (!body || typeof body !== "object" || Array.isArray(body)) return { ok: false, reason: "failed" };
  const declarations: Declarations = {};
  for (const section of ["networks", "volumes"] as const) {
    const value = doc?.[section];
    if (value === undefined || value === null) continue;
    if (typeof value !== "object" || Array.isArray(value)) return { ok: false, reason: "failed" };
    declarations[section] = value as Record<string, unknown>;
  }
  // Other top-level sections (configs, secrets) cannot be imported losslessly as one service.
  if (Object.keys(doc ?? {}).some((k) => !["services", "networks", "volumes", "name"].includes(k))) return { ok: false, reason: "failed" };
  return { ok: true, declarations, conv: { key: uniqueKey(existing, serviceName || "app"), body: body as Record<string, unknown>, hints: [] }, env };
}
