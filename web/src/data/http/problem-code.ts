// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original shared problem-code decoder for hub, agent and maintenance answers.

const CODE = /^[a-z][a-z0-9_-]*(\.[a-z0-9_-]+)+$/;

/** The hub's maintenance gate answers 503 problem+json with the code in `detail`. */
export const MAINTENANCE_CODE = "backup.maintenance";

/** The machine code of an answer body: an enveloped `data.code`, `code`, or a problem+json `detail` that is a code
 * (prose details are ignored). */
export function problemCode(body: unknown): string | undefined {
  if (!body || typeof body !== "object") return undefined;
  const value = body as Record<string, unknown>;
  const data = value.data && typeof value.data === "object" ? value.data as Record<string, unknown> : undefined;
  for (const entry of [data?.code, value.code]) if (typeof entry === "string" && entry) return entry;
  return typeof value.detail === "string" && CODE.test(value.detail) ? value.detail : undefined;
}
