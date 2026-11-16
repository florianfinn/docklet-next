// Source: docklet-next/issues/465
// Revision: b4f894ff7f35186d4c39be1fc9fccd01b4b5934a
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original bounded runtime blocker DTO decoding.

import type { components } from "@/api/generated/schema";

export type RuntimeIdentityBlocker = components["schemas"]["ImageRuntimeIdentityBlocker"];

export function runtimeBlocker(code: unknown, value: unknown): RuntimeIdentityBlocker | undefined {
 if (code !== "runtime.target-unavailable" || !value || typeof value !== "object" || Array.isArray(value)) return;
 const blocker = value as Record<string, unknown>;
 if (typeof blocker.name !== "string" || !blocker.name.trim() || new TextEncoder().encode(blocker.name).length > 128 || /[/\\:]/u.test(blocker.name) || /[\p{Cc}\p{White_Space}]/u.test(blocker.name.replaceAll(" ", "")) ||
     (blocker.reason !== "compose.selection-unsupported" && blocker.reason !== "compose.identity-unreadable") || typeof blocker.archived !== "boolean") return;
 return { name: blocker.name, reason: blocker.reason, archived: blocker.archived };
}
