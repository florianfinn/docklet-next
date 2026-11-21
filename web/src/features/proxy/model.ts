// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import type {
  Acknowledgements,
  Change,
  Instance,
  Inventory,
  OperationRequest,
  ProxyFile,
  ReadResult,
  Route,
} from "./types";

export const emptyAcknowledgements = (): Acknowledgements => ({
  unresolvedReferences: [],
  replaceRunningConfig: false,
  restart: false,
});
export const canEdit = (r: Route) => r.class === "editable" && r.eligibility.writable && !r.eligibility.textOnly;
export const drifted = (f: ProxyFile) => f.owned && !!f.lastVerifiedHash && f.hash !== f.lastVerifiedHash;
export const canInsertImport = (i: Inventory) =>
  !i.importPresent && !!i.importLine && i.eligibility.writable && i.eligibility.reason === "import-missing";
export const routeWritesAllowed = (read: ReadResult, inventory: Inventory) =>
  read.eligibility.writable && inventory.eligibility.writable && inventory.eligibility.reason !== "import-missing";
export function acknowledgementsReady(inventory: Inventory, unresolved: string[], ack: Acknowledgements) {
  return (
    (!inventory.runningDiverged || ack.replaceRunningConfig) &&
    (!inventory.adminOff || ack.restart) &&
    unresolved.every(name => ack.unresolvedReferences?.includes(name))
  );
}
/** Targets sent by engines are source-relative, even when setup locations are absolute container paths. */
// The backend accepts only clean source-relative paths, so "." segments go.
export function relativeTarget(i: Instance, location: string): string {
  const prefix = i.source.mountTarget.replace(/\/$/, "") + "/";
  const relative = location.startsWith(prefix) ? location.slice(prefix.length) : location;
  return relative.split("/").filter((part, n) => part !== "." && (part !== "" || n === 0)).join("/");
}
export function operationRequest(
  read: ReadResult,
  target: string,
  change: Change,
  acknowledgements: Acknowledgements,
): OperationRequest {
  const current = (read.files ?? []).find(f => f.path === target);
  return {
    target,
    expected: { present: !!current, hash: current?.hash ?? "" },
    graphHashes: Object.fromEntries([
      ...(read.files ?? []).map(f => [f.path, f.hash]),
      ...(read.opaqueFiles ?? []).map(f => [f.path, "opaque"]),
    ]),
    change,
    acknowledgements,
  };
}
export const splitNames = (value: string) =>
  value.split("\n").filter(x => x !== "");
/** Split a backend URL as written: the port and any path stay byte for byte; Caddy may omit the scheme. */
export function backendParts(value: string) {
  const m = /^(?:([A-Za-z][A-Za-z0-9+.-]*):\/\/)?(\[[^\]]*\]|[^/:?#]*)(?::(\d*))?(.*)$/.exec(value);
  return value && m
    ? { scheme: m[1] ?? "", host: m[2]!, port: m[3] ?? "", colon: m[3] !== undefined, rest: m[4]! }
    : { scheme: "http", host: "", port: "", colon: false, rest: "" };
}
/** Rebuild a backend URL; with the lossless split above an untouched value comes back byte for byte. */
export function backendURL(parts: ReturnType<typeof backendParts>) {
  // An explicit empty port (`host:`) keeps its separator until the port itself is edited.
  return `${parts.scheme ? `${parts.scheme}://` : ""}${parts.host}${parts.port || parts.colon ? `:${parts.port}` : ""}${parts.rest}`;
}
