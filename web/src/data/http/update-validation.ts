// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original phase 2a stack update consumer for issue #231.

import { AdapterOutcomeError } from "../capability";
import type { components } from "@/api/generated/schema";
import { enBackend } from "../messages/en";
export const updateReasons = new Set(Object.entries(enBackend.errors).flatMap(([prefix, entries]) => typeof entries === "string" ? [prefix] : Object.keys(entries).map((key) => `${prefix}.${key}`)));

updateReasons.add("host.restarting");

export const invalidUpdate = () => new AdapterOutcomeError({ kind: "error", code: "http.error" });
export function record(value: unknown): Record<string, unknown> {
  if (!value || typeof value !== "object" || Array.isArray(value)) throw invalidUpdate();
  return value as Record<string, unknown>;
}
export function text(value: unknown): string { if (typeof value !== "string") throw invalidUpdate(); return value; }
export function id(value: unknown): string { const s = text(value); if (!s.trim()) throw invalidUpdate(); return s; }
export function bool(value: unknown): boolean { if (typeof value !== "boolean") throw invalidUpdate(); return value; }
export function revision(value: unknown): number { if (!Number.isSafeInteger(value) || Number(value) < 0) throw invalidUpdate(); return Number(value); }
export function date(value: unknown): string { const s = id(value); if (!Number.isFinite(Date.parse(s))) throw invalidUpdate(); return s; }
export function optionalDate(value: unknown): string | undefined { return value === undefined || value === null ? undefined : date(value); }
export function list(value: unknown): unknown[] { if (value === null) return []; if (!Array.isArray(value)) throw invalidUpdate(); return value; }
export function choice<const T extends readonly string[]>(value: unknown, allowed: T): T[number] { const s = text(value); if (!allowed.includes(s)) throw invalidUpdate(); return s; }
/** The enum is declared on the policy DTO. Unknown strings have a declared generic presentation. */
export function reason(value: unknown): string | undefined {
  if (value === undefined || value === "") return undefined;
  const code = text(value);
  return updateReasons.has(code) ? code : "http.error";
}
export type Observation = components["schemas"]["ImageUpdateInfo"];
export function observation(value: unknown): Observation {
  const o = record(value);
  choice(o.state, ["unchecked", "current", "version", "rebuild", "local", "unknown", "error"]);
  choice(o.authState, ["anonymous", "credential", "authentication-required", "authentication-failed", "unknown"]);
  id(o.imageRef); date(o.checkedAt);
  for (const field of ["currentVersion", "latestVersion", "currentDigest", "latestDigest", "error", "updateType"]) text(o[field]);
  if (bool(o.hasUpdate) !== (o.state === "version" || o.state === "rebuild")) throw invalidUpdate();
  if (typeof o.responseTimeMs !== "number" || !Number.isFinite(o.responseTimeMs)) throw invalidUpdate();
  if (o.platform !== undefined) text(o.platform);
  if (o.authRegistry !== undefined) text(o.authRegistry);
  return { ...o, code: reason(o.code), error: "" } as unknown as Observation;
}
