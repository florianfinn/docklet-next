// Source: docklet-next/docs/design/runtime-control.md
// Revision: d4229b124f079376c959142b63235f6a739be2a9711e0228ec5a63ab552f8cf0
// Retrieved: 2026-11-20
// License: Apache-2.0
// Deviations: Original peer identity confirmation contract for issue 531.

import type { MessageCode } from "../message-code";
import { runtimeBlocker, runtimeReasonKey, safeRuntimeFile } from "../runtime-blocker";

export function runtimePeerAudit(code: string | undefined, value: unknown): MessageCode | undefined {
 if (code !== "runtime.peer-acknowledged") return;
 const unavailable: MessageCode = { code: "common.runtimePeers.auditUnavailable", params: {} };
 if (!value || typeof value !== "object") return unavailable;
 const payload = value as Record<string, unknown>;
 if (!Number.isSafeInteger(payload.count) || (payload.count as number) < 1 || !Array.isArray(payload.peers) || payload.peers.length !== Math.min(payload.count as number, 16)) return unavailable;
 const params: MessageCode["params"] = { count: payload.count as number };
 for (const [i, value] of payload.peers.entries()) {
  if (!value || typeof value !== "object") return unavailable;
  const peer = value as Record<string, unknown>;
  if (!["compose.identity-unreadable", "compose.selection-unsupported", "compose.env-file-missing"].includes(peer.reason as string)) return unavailable;
  const valid = runtimeBlocker("runtime.target-unavailable", { name: peer.name, reason: peer.reason, archived: false });
  if (!valid || peer.file !== undefined && peer.file !== "" && !safeRuntimeFile(peer.file)) return unavailable;
  params[`peer${i}`] = { code: peer.file ? "common.runtimePeers.auditPeerFile" : "common.runtimePeers.auditPeer", params: { name: valid.name, reason: { code: `common.runtimePeers.${runtimeReasonKey(valid.reason)}`, params: {} }, ...(peer.file ? { file: peer.file as string } : {}) } };
 }
 return { code, params };
}
