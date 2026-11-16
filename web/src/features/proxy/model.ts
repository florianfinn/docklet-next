// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import type { Acknowledgements, Change, Instance, Inventory, OperationRequest, ProxyFile, ReadResult, Route } from "./types";

export const emptyAcknowledgements = (): Acknowledgements => ({ unresolvedReferences: [], replaceRunningConfig: false, restart: false });
export const canEdit = (r: Route) => r.class === "editable" && r.eligibility.writable && !r.eligibility.textOnly;
export const drifted = (f: ProxyFile) => f.owned && !!f.lastVerifiedHash && f.hash !== f.lastVerifiedHash;
export const canInsertImport = (i: Inventory) => !i.importPresent && !!i.importLine && i.eligibility.writable && i.eligibility.reason === "import-missing";
export const routeWritesAllowed = (read: ReadResult, inventory: Inventory) => read.eligibility.writable && inventory.eligibility.writable && inventory.eligibility.reason !== "import-missing";
export function acknowledgementsReady(inventory: Inventory, unresolved: string[], ack: Acknowledgements) {
  return (!inventory.runningDiverged || ack.replaceRunningConfig) && (!inventory.adminOff || ack.restart) && unresolved.every(name => ack.unresolvedReferences?.includes(name));
}
/** Targets sent by engines are source-relative, even when setup locations are absolute container paths. */
export function relativeTarget(i: Instance, location: string): string {
  const prefix = i.source.mountTarget.replace(/\/$/, "") + "/";
  return location.startsWith(prefix) ? location.slice(prefix.length) : location;
}
export function operationRequest(read: ReadResult, target: string, change: Change, acknowledgements: Acknowledgements): OperationRequest {
  const current = (read.files ?? []).find(f => f.path === target);
  return { target, expected: { present: !!current, hash: current?.hash ?? "" }, graphHashes: Object.fromEntries([...(read.files ?? []).map(f => [f.path, f.hash]), ...(read.opaqueFiles ?? []).map(f => [f.path, "opaque"])]), change, acknowledgements };
}
export const splitNames = (value: string) => value.split(/[\n,]/).map(x => x.trim()).filter(Boolean);
export function backendParts(value: string) {
  try { const u = new URL(value); return { scheme: u.protocol.slice(0, -1), host: u.hostname, port: u.port }; }
  catch { return { scheme: "http", host: "", port: "" }; }
}
