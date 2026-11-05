// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original phase 2a stack update consumer for issue #231.

import { AdapterOutcomeError } from "../capability";
import type { UpdateBaseline, UpdatePoint, StackUpdatePolicy, SystemHints } from "@/domain/stack-updates";
import { record, text, id, bool, revision, date, optionalDate, list, choice, reason, invalidUpdate } from "./update-validation";
import { observation } from "./update-validation";
export function baseline(value: unknown): UpdateBaseline {
  const o = record(value);
  for (const key of ["targetIdentity", "savedRevision", "observedRevision"]) if (!/^[a-f0-9]{64}$/.test(text(o[key]))) throw invalidUpdate();
  return { targetIdentity: id(o.targetIdentity), savedRevision: text(o.savedRevision), observedRevision: text(o.observedRevision), available: bool(o.available), autoExcluded: bool(o.autoExcluded), code: reason(o.code), ...(o.sourceRunId === undefined ? {} : { sourceRunId: id(o.sourceRunId) }) };
}
export function points(value: unknown, target?: string): UpdatePoint[] {
  try { return list(value).map((v) => {
    const o = record(v);
    if (!Array.isArray(o.images)) throw invalidUpdate();
    if ((target !== undefined && id(o.targetId) !== target) || bool(o.volumeDataIncluded)) throw invalidUpdate();
    return { id: id(o.id), registrationId: id(o.registrationId), targetId: id(o.targetId), targetIdentity: id(o.targetIdentity), sourceRunId: id(o.sourceRunId), createdAt: date(o.createdAt), sourceState: choice(o.sourceState, ["settled", "partial-source"]), available: bool(o.available), orphaned: bool(o.orphaned), volumeDataIncluded: false, code: reason(o.code), protectionReason: o.protectionReason === null ? null : choice(o.protectionReason, ["restore-target", "healthy-baseline"]), holdState: choice(o.holdState, ["none", "active", "unknown"]), failedAttempt: bool(o.failedAttempt), images: list(o.images).map((v) => { const i = record(v); if ((bool(i.present) && !/^sha256:[a-f0-9]{64}$/.test(text(i.imageId))) || (i.digest !== undefined && text(i.digest) !== "" && !/^sha256:[a-f0-9]{64}$/.test(text(i.digest)))) throw invalidUpdate(); return { service: id(i.service), reference: bool(i.present) ? id(i.reference) : text(i.reference), imageId: bool(i.present) ? id(i.imageId) : text(i.imageId), ...(i.digest === undefined ? {} : { digest: text(i.digest) }), present: bool(i.present), running: bool(i.running) }; }) };
  }); } catch { throw new AdapterOutcomeError({ kind: "error", code: "update.check-unavailable" }); }
}
export function policy(value: unknown, host: string, target: string, kind: "stack" | "container" = "stack"): StackUpdatePolicy {
  const o = record(value);
  if ((o.mode === "scheduled") !== (o.window !== undefined)) throw invalidUpdate();
  // The producer sends a stack's baseline hash, a container's name, or "" before a target was bound (updater GetImagePolicy).
  const identity = text(o.targetIdentity);
  if (kind === "stack" && identity !== "" && !/^[a-f0-9]{64}$/.test(identity)) throw invalidUpdate();
  if (o.environmentId !== host || o.targetId !== target || o.targetKind !== kind) throw invalidUpdate();
  return { mode: choice(o.mode, ["check", "scheduled"]), revision: revision(o.revision), environmentId: host, registrationId: id(o.registrationId), targetKind: kind, targetName: text(o.targetName), targetMissing: bool(o.targetMissing), window: o.window === undefined ? undefined : window(o.window), targetId: id(o.targetId), targetIdentity: identity, jobId: text(o.jobId), stale: bool(o.stale),
    checkedAt: optionalDate(o.checkedAt), code: reason(o.code), applyCheckedAt: optionalDate(o.applyCheckedAt), applyCode: reason(o.applyCode), bookkeepingAt: optionalDate(o.bookkeepingAt), bookkeepingCode: reason(o.bookkeepingCode),
    candidates: list(o.candidates).map((v) => { const c = record(v); return { service: id(c.service), imageRef: id(c.imageRef), version: text(c.version), digest: id(c.digest), platform: id(c.platform), ...(c.entryId === undefined ? {} : { entryId: id(c.entryId) }), notificationState: choice(c.notificationState, ["pending", "sending", "sent", "unconfigured", "failed", "not-requested"]) }; }) };
}
export function window(value: unknown): import("@/domain/stack-updates").UpdateWindow {
  const o = record(value), kind = choice(o.kind, ["daily", "weekly"]), time = text(o.time);
  if (!/^([01]\d|2[0-3]):[0-5]\d$/.test(time)) throw invalidUpdate();
  if (kind === "daily") return { kind, time };
  const day = revision(o.day); if (day > 6) throw invalidUpdate();
  return { kind, time, day };
}
function version(v: string): number[] | undefined {
  v = v.split("+", 1)[0]!;
  if (!/^v?(0|[1-9]\d*)\.(0|[1-9]\d*)\.(0|[1-9]\d*)$/.test(v)) return undefined;
  const parts = v.replace(/^v/, "").split(".").map(Number);
  return parts.every(Number.isSafeInteger) ? parts : undefined;
}
export function compareVersion(a: string, b: string): number | undefined { const x = version(a), y = version(b); if (!x || !y) return undefined; for (let i = 0; i < 3; i++) if (x[i] !== y[i]) return x[i]! - y[i]!; return 0; }
export function hints(value: unknown): SystemHints {
  const o = record(value), hubVersion = text(o.hubVersion);
  return { hubVersion, images: list(o.images).map((v) => { const h = record(v); const kind = choice(h.kind, ["agent", "tools"]); const currentVersion = h.currentVersion === undefined ? undefined : text(h.currentVersion); const candidateVersion = h.candidateVersion === undefined ? undefined : text(h.candidateVersion);
    if (bool(h.manualOnly) !== true) throw invalidUpdate();
    if (kind === "agent" && candidateVersion && (compareVersion(candidateVersion, hubVersion) === undefined || compareVersion(candidateVersion, hubVersion)! > 0 || !currentVersion || compareVersion(candidateVersion, currentVersion) === undefined || compareVersion(candidateVersion, currentVersion)! <= 0)) throw invalidUpdate();
    return { kind, reference: text(h.reference), currentVersion, candidateVersion, compatibleReference: h.compatibleReference === undefined ? undefined : text(h.compatibleReference), state: choice(h.state, ["current", "available", "pinned", "unknown"]), requiresHubUpdate: bool(h.requiresHubUpdate), manualOnly: true, code: reason(h.code) }; }) };
}
export function checkResult(value: unknown) {
  const o = record(value); id(o.status); bool(o.hasUpdate);
  for (const k of ["imageCount", "checkedImageCount", "imagesWithUpdates", "imagesNotPulled", "errorCount"]) revision(o[k]);
  for (const [name, raw] of Object.entries(o.serviceUpdates === undefined ? {} : record(o.serviceUpdates))) { id(name); const service = record(raw); id(service.imageRef); if (service.updateInfo !== undefined) observation(service.updateInfo); }
  for (const raw of Object.values(o.updateInfoByRef === undefined ? {} : record(o.updateInfoByRef))) observation(raw);
  reason(o.policyCode); optionalDate(o.lastCheckedAt);
}
