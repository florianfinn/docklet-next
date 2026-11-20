// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import type { components } from "@/api/generated/schema";

export type RuntimeIdentityBlocker = components["schemas"]["ImageRuntimeIdentityBlocker"];
export type RuntimePeer = components["schemas"]["ImageRuntimeIdentityPeer"];
const bytes = (value: string) => new TextEncoder().encode(value).length;
const object = (value: unknown): Record<string, unknown> => value && typeof value === "object" && !Array.isArray(value) ? value as Record<string, unknown> : {};
const safeName = (value: unknown): value is string => typeof value === "string" && !!value.trim() && bytes(value) <= 128 && !/[/\\:]/u.test(value) && !/[\p{Cc}\p{White_Space}]/u.test(value.replaceAll(" ", "")) && !/[\uD800-\uDFFF]/u.test(value);
export const safeRuntimeFile = (value: unknown): value is string => typeof value === "string" && !!value && bytes(value) <= 255 && !/[\\:\p{Cc}\uD800-\uDFFF]/u.test(value) && value.split("/").every((part) => part !== "" && part !== "." && part !== "..");
const safeID = (value: unknown): value is string => typeof value === "string" && /^[a-zA-Z0-9_-]{1,256}$/.test(value);
const reason = (value: unknown) => value === "compose.selection-unsupported" || value === "compose.identity-unreadable" || value === "compose.env-file-missing";
export const runtimeReasonKey = (value: string) => value === "compose.selection-unsupported" ? "selection" : value === "compose.env-file-missing" ? "missing" : "identity";

export function runtimeBlocker(code: unknown, value: unknown): RuntimeIdentityBlocker | undefined {
 if (code !== "runtime.target-unavailable") return;
 const b = object(value);
 if (!safeName(b.name) || (!reason(b.reason) && b.reason !== "compose.name-undeterminable" && b.reason !== "compose.duplicate") || typeof b.archived !== "boolean") return;
 const result: RuntimeIdentityBlocker = { name: b.name, reason: b.reason as RuntimeIdentityBlocker["reason"], archived: b.archived };
 if (b.peerId !== undefined && !safeID(b.peerId) || b.file !== undefined && !safeRuntimeFile(b.file)) return result;
 if (safeID(b.peerId)) result.peerId = b.peerId;
 if (safeRuntimeFile(b.file)) result.file = b.file;
 const u = object(b.uncertainty);
 if (b.confirmable !== true || b.reason !== "compose.name-undeterminable" || typeof u.digest !== "string" || !/^[a-f0-9]{64}$/i.test(u.digest) || !Number.isSafeInteger(u.count) || (u.count as number) < 1 || !Array.isArray(u.peers) || u.peers.length !== Math.min(u.count as number, 16)) return result;
 const peers: RuntimePeer[] = [], seen = new Set<string>();
 for (const value of u.peers) {
  const p = object(value);
  if (!safeID(p.peerId) || seen.has(p.peerId) || !safeName(p.name) || !reason(p.reason) || typeof p.archived !== "boolean" || p.file !== undefined && !safeRuntimeFile(p.file)) return result;
  seen.add(p.peerId);
  peers.push({ peerId: p.peerId, name: p.name, reason: p.reason as RuntimePeer["reason"], archived: p.archived, ...(p.file ? { file: p.file as string } : {}) });
 }
 return { ...result, confirmable: true, uncertainty: { digest: u.digest.toLowerCase(), count: u.count as number, peers } };
}
