// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { AdapterOutcomeError } from "@/data/capability";

export const record = (value: unknown): Record<string, unknown> => value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
/** Image/CRUD errors nest error.code. Gateway rejections have a separate top-level code. */
export function imageErrorCode(body: unknown): string | undefined {
  const value = record(body);
  const code = record(value.error).code;
  return typeof code === "string" ? code : typeof value.code === "string" ? value.code : undefined;
}
export function imageResponseError(body: unknown, status?: number, write = true): AdapterOutcomeError {
  const code = imageErrorCode(body);
  return new AdapterOutcomeError({ kind: "error", status, code: write && (status === 502 || status === 504) && code === "agent.unavailable" ? "image.outcome-unknown" : code ?? "http.error" });
}
export function imageUnknown(): AdapterOutcomeError {
  return new AdapterOutcomeError({ kind: "error", code: "image.outcome-unknown" });
}
