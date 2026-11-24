// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-16
// License: Apache-2.0
// Deviations: Original production proxy Manager implementation.

import {PROXY_ADOPTION_REASON_KEYS, PROXY_ADOPTION_OUTCOME_KEYS} from "../wire-catalogue-proxy";
import type {components} from "@/api/generated/schema";
import {validProxySourceExtension, validResolverSummary} from "./proxy-decoder";
import type { Discovery, Instance, Inventory, ReadResult } from "@/features/proxy/types";

const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const text = (v: unknown) => typeof v === "string";
const list = (v: unknown, check: (x: unknown) => boolean): boolean =>
  v === null || (Array.isArray(v) && v.every(check));
const member = (v: unknown, values: string[]) => text(v) && values.includes(v as string);
const identity = (v: unknown) => record(v) && [v.containerId, v.containerName, v.startedAt].every(text);
const eligibility = (v: unknown) =>
  record(v) &&
  typeof v.writable === "boolean" &&
  typeof v.textOnly === "boolean" &&
  [v.reason, v.path, v.detail].every(x => x === undefined || text(x)) &&
  (v.line === undefined || Number.isInteger(v.line));
const source = (v: unknown) =>
  record(v) &&
  member(v.kind, ["project", "volume", "bind", "single-file", "unknown"]) &&
  [v.canonical, v.mountTarget].every(text) &&
  typeof v.readOnly === "boolean" && validProxySourceExtension(v);
const discovery = (v: unknown) =>
  record(v) &&
  identity(v.identity) &&
  member(v.kind, ["traefik", "caddy"]) &&
  text(v.image) &&
  list(v.sources, source) &&
  (v.instanceId === undefined || text(v.instanceId));
const instance = (v: unknown) =>
  record(v) &&
  [v.id, v.environmentId, v.name, v.routeDirectory, v.generation, v.registration].every(text) &&
  identity(v.identity) &&
  source(v.source) &&
  eligibility(v.eligibility) &&
  member(v.kind, ["traefik", "caddy"]) &&
  member(v.authority, ["file", "caddyfile", "admin-api"]);
const inventory = (v: unknown) =>
  record(v) &&
  identity(v.identity) &&
  text(v.generation) &&
  eligibility(v.eligibility) &&
  [v.watch, v.adminOff, v.resume, v.persistConfig, v.runningDiverged, v.importPresent].every(
    x => typeof x === "boolean",
  ) &&
  Number.isFinite(v.throttleMillis) &&
  [v.resolvers, v.providers, v.plugins, v.imports].every(x => list(x, text)) &&
  (v.resolverSummaries === undefined || list(v.resolverSummaries, validResolverSummary)) &&
  list(v.entryPoints, x => record(x) && text(x.name) && text(x.address) && typeof x.tls === "boolean");
const file = (v: unknown) =>
  record(v) &&
  [v.path, v.hash, v.content, v.modifiedAt].every(text) &&
  typeof v.owned === "boolean" &&
  typeof v.mode === "number";
const reference = (x: unknown) =>
  record(x) &&
  text(x.name) &&
  member(x.kind, ["local", "plugin", "internal", "external", "opaque", "unresolved"]) &&
  (x.file === undefined || text(x.file));
const route = (v: unknown) =>
  record(v) &&
  [v.id, v.name, v.file, v.rule, v.service].every(text) &&
  (v.tls === undefined || typeof v.tls === "boolean") &&
  (v.resolver === undefined || text(v.resolver)) &&
  member(v.class, ["editable", "text-only", "infrastructure", "label-read-only"]) &&
  eligibility(v.eligibility) &&
  Number.isInteger(v.priority) &&
  [v.upstreams, v.entryPoints, v.dependants].every(x => list(x, text)) &&
  list(v.references, reference);
const read = (v: unknown) =>
  record(v) &&
  text(v.generation) &&
  inventory(v.inventory) &&
  eligibility(v.eligibility) &&
  list(v.files, file) &&
  list(v.routes, route) &&
  list(v.opaqueFiles, x => record(x) && text(x.path) && Object.keys(x).every(k => k === "path")) &&
  (v.middlewares === undefined || list(v.middlewares, reference));
type Results = {
  instances: Instance[];
  instance: Instance;
  discovery: Discovery[];
  inventory: Inventory;
  read: ReadResult;
};
const checks = {
  instances: (v: unknown) => list(v, instance),
  instance,
  discovery: (v: unknown) => list(v, discovery),
  inventory,
  read,
};
export function decodeProxyData<K extends keyof Results>(kind: K, value: unknown): Results[K] {
  if (!record(value) || value.success !== true || !checks[kind](value.data)) throw new Error("proxy.invalid-response");
  return (value.data ?? []) as Results[K];
}

export function decodeProxyAdoption(value: unknown): components["schemas"]["ProxyAdoption"] {
 if (!record(value) || value.success!==true || !record(value.data)) throw new Error("proxy.invalid-response");
 const v=value.data;
 if (!member(v.outcome,Object.keys(PROXY_ADOPTION_OUTCOME_KEYS)) || !instance(v.instance) ||
  (v.reason!==undefined && !member(v.reason,Object.keys(PROXY_ADOPTION_REASON_KEYS))) ||
  (v.outcome==="refused" && v.reason===undefined) || (v.outcome!=="refused" && v.reason!==undefined) ||
  (v.supersededBy!==undefined && (v.reason!=="held-by-instance" || typeof v.supersededBy!=="string" || !v.supersededBy)) ||
  (v.reason==="held-by-instance" && !v.supersededBy) ||
  (v.outcome==="adopted" && (typeof v.operationId!=="string" || !v.operationId)) ||
  (v.operationId!==undefined && (v.outcome!=="adopted" || typeof v.operationId!=="string"))) throw new Error("proxy.invalid-response");
 return v as components["schemas"]["ProxyAdoption"];
}
