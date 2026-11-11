// Source: docklet-next/issues/425
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original reader of the hub's non-secret session presence hint.

// The hub sets and clears this hint together with its HttpOnly session cookie (`__Host-` when secure); it is never
// authorization, only a reason to probe the session.
const NAMES = ["__Host-docklet_session", "docklet_session"];

/** Whether the browser may hold a session; without a document (tests, workers) a session is assumed. */
export function hasSessionHint(): boolean {
  if (typeof document === "undefined") return true;
  return document.cookie.split(";").some((part) => NAMES.includes(part.trim().split("=")[0] ?? ""));
}

/** Drop a hint that outlived its session, so the next start does not probe again. */
export function clearSessionHint(): void {
  if (typeof document === "undefined") return;
  document.cookie = "__Host-docklet_session=; Path=/; Max-Age=0; Secure; SameSite=Lax";
  document.cookie = "docklet_session=; Path=/; Max-Age=0; SameSite=Lax";
}
