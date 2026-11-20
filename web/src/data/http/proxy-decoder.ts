// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original generated-contract proxy outcome decoder.

import {PROXY_STATE_KEYS,PROXY_REASON_KEYS} from "../wire-catalogue-proxy";
import {wireKey,type CatalogueKey} from "../wire-catalogue";
import type { components } from "@/api/generated/schema";
export type ProxyOperation = components["schemas"]["ProxyOperation"];
export type ProxyReason = components["schemas"]["ProxyProblem"]["code"];
const states = ["pending", "verified", "applied-unverified", "rejected", "restored", "recovery-required", "released", "not-applied", "applied", "rolled-back", "refused"] as const satisfies readonly ProxyOperation["state"][];
const phases = ["admitted", "staging", "publishing", "reloading", "restarting", "observing", "marking", "restoring","backup","writing","recreating","applied","rolled-back","refused","recovery-required"] as const satisfies readonly ProxyOperation["phase"][];
const reasons = ["no-observable-difference", "not-observable", "container-changed", "logs-unavailable", "generation-mismatch", "operation-conflict", "module-busy", "stale-hash", "source-read-only", "unknown-operation", "operation-fenced", "registration-changed", "journal-lost", "invalid-input", "input-limit", "exec-failed", "exec-running", "proxy-rejected", "restore-failed", "invalid-graph", "incomplete-inventory", "opaque-input", "single-file-mount", "refused-bind", "read-only-mount", "unknown-source", "unsupported-metadata", "watch-off", "unsupported-format", "template-input", "rule-syntax-v2", "shared-across-files", "persistence-off", "relative-import", "import-missing", "acknowledgement-required", "probe-unavailable", "probe-option-unavailable", "admin-unavailable", "agent-unavailable", "engine-unavailable", "target-changed","source-ineligible","route-directory-mismatch","provider-directory-elsewhere","caddy-config-implicit","read-only","route-changed","certificate-unavailable","probe-image-unavailable","probe-client-missing","probe-client-unsupported","third-version","backup-invalid","unsupported-layout","env-conflict","env-key-exists","env-duplicate","storage-not-writable","unknown-operation-kind","compose-load-failed","compose-up-failed","static-parse-failed","effect-unsettled","target-unclear","identity-unclear","interrupted","not-implemented"] as const satisfies readonly ProxyReason[];
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const member = (items: readonly string[], v: unknown) => typeof v === "string" && items.includes(v);
// Diagnostic is optional engine text; its bounds come from the wire contract.
function diagnostic(v: unknown): boolean { return record(v) && (v.path === undefined || (typeof v.path === "string" && v.path.length <= 1024)) && (v.line === undefined || (Number.isInteger(v.line) && (v.line as number) >= 0)) && (v.detail === undefined || (typeof v.detail === "string" && v.detail.length <= 4096)); }
function fileState(v: unknown): boolean { return record(v) && typeof v.present === "boolean" && typeof v.hash === "string" && (v.present ? /^[a-f0-9]{64}$/.test(v.hash) : v.hash === ""); }
export function decodeProxyOperation(body: unknown): ProxyOperation {
  if (!record(body) || body.success !== true || !record(body.data)) throw new Error("proxy.invalid-response");
  const v = body.data;
  if (!member(states, v.state) || !member(phases, v.phase) || (v.reason !== undefined && v.reason !== "" && !member(reasons, v.reason)) || (v.evidence !== undefined && !member(["routing", "configuration", "load"], v.evidence))) throw new Error("proxy.invalid-response");
  if (v.kind!==undefined && (typeof v.kind!=="string" || v.kind==="" || (!["route","resolver"].includes(v.kind) && !(["recovery-required","released"].includes(v.state as string)&&v.reason==="unknown-operation-kind")))) throw new Error("proxy.invalid-response");
  if (!validStateForKind(v.kind,v.state)) throw new Error("proxy.invalid-response");
  if ((v.certificate!==undefined&&!certificate(v.certificate)) || (v.probe!==undefined&&!probe(v.probe)) || (v.resolver!==undefined&&!resolverReceipt(v.resolver)) || (v.kind==="resolver"&&v.diagnostic!==undefined)) throw new Error("proxy.invalid-response");
  if (!(typeof v.target==="string" && (v.target!=="" || v.kind==="resolver" || v.reason==="unknown-operation-kind"))) throw new Error("proxy.invalid-response");
  if (!["id", "instanceId", "generation", "registration", "digest", "createdAt"].every((k) => typeof v[k] === "string" && v[k] !== "") || !fileState(v.old) || !fileState(v.new) || (v.current !== undefined && !fileState(v.current)) || (v.diagnostic !== undefined && !diagnostic(v.diagnostic))) throw new Error("proxy.invalid-response");
  return v as ProxyOperation;
}
// Presentation retains route verification and resolver application as distinct outcomes.
export function proxyOutcome(operation: ProxyOperation): { state: ProxyOperation["state"]; tone: "success" | "warning" | "danger" | "neutral"; reason: string | undefined;labelKey:CatalogueKey;reasonKey:CatalogueKey|undefined } {
  const state = operation.state;
  const tone = state === "verified" || state === "applied" ? "success" : state === "recovery-required" || state === "rejected" || state === "refused" ? "danger" : state === "applied-unverified" || state === "restored" || state === "rolled-back" ? "warning" : "neutral";
  return { state, tone, reason: operation.reason,labelKey:PROXY_STATE_KEYS[state],reasonKey:wireKey(PROXY_REASON_KEYS,operation.reason) };
}

const text = (v: unknown): v is string => typeof v === "string";
const timestamp = (v: unknown) => text(v) && Number.isFinite(Date.parse(v));
const sha256 = (v: unknown) => text(v) && /^[a-f0-9]{64}$/.test(v);
function certificate(v: unknown): boolean {
 if (!record(v) || !record(v.epoch) || !record(v.route) || !record(v.peer)) return false;
 return timestamp(v.observedAt) && text(v.instanceId) && v.instanceId!=="" && text(v.epoch.containerId) && v.epoch.containerId!=="" && timestamp(v.epoch.startedAt) &&
 [v.route.name,v.route.host,v.route.sni,v.route.entryPoint].every(text) &&
 [v.peer.subject,v.peer.issuer].every(x=>text(x)&&x.length<=256) && Array.isArray(v.peer.sans) && v.peer.sans.length<=32 && v.peer.sans.every(x=>text(x)&&x.length<=256) && timestamp(v.peer.notBefore) && timestamp(v.peer.notAfter) && sha256(v.peer.sha256) &&
 typeof v.chainTrusted === "boolean" && typeof v.hostnameMatch === "boolean" && member(["valid","expired","not-yet-valid"],v.validity) && (v.error===undefined || (text(v.error)&&v.error.length<=256));
}
function probe(v: unknown): boolean {
 return record(v) && member(["helper","wget"],v.client) && (v.imageId===undefined || text(v.imageId)) && (v.client!=="helper" || (text(v.imageId)&&v.imageId!=="")) && (v.reason===undefined || member(reasons,v.reason));
}
export function validProxySourceExtension(v: Record<string,unknown>): boolean {
 return (v.mountMode===undefined || member(["rw","ro"],v.mountMode)) && (v.projectPath===undefined || text(v.projectPath)) &&
 (v.eligibility===undefined || (record(v.eligibility) && typeof v.eligibility.selectable==="boolean" && (v.eligibility.reason===undefined || member(reasons,v.eligibility.reason)) &&
 (v.eligibility.rootFile===undefined || text(v.eligibility.rootFile)) &&
 (v.eligibility.routeDirectory===undefined || (record(v.eligibility.routeDirectory) && text(v.eligibility.routeDirectory.value) && typeof v.eligibility.routeDirectory.fixed==="boolean"))));
}
export function validResolverSummary(v: unknown): boolean {
 return resolverConfig(v) && record(v) && Array.isArray(v.variables) && v.variables.every(x=>record(x)&&text(x.name)&&typeof x.configured==="boolean"&&!("value" in x));
}
function resolverConfig(v: unknown): boolean {
 if (!record(v) || !text(v.name) || (v.email!==undefined&&!text(v.email)) || !text(v.storage) || !record(v.ca) || !record(v.challenge)) return false;
 return member(["letsencrypt","letsencrypt-staging","custom"],v.ca.kind) && (v.ca.url===undefined||text(v.ca.url)) && member(["http","dns"],v.challenge.kind) &&
 (v.challenge.entryPoint===undefined||text(v.challenge.entryPoint)) && (v.challenge.provider===undefined||member(["cloudflare","hetzner","ionos","inwx","netcup","desec","duckdns","httpreq"],v.challenge.provider)) &&
 Number.isInteger(v.challenge.delayBeforeCheck) && (v.challenge.delayBeforeCheck as number)>=0 && (v.challenge.delayBeforeCheck as number)<=600 && (v.challenge.resolvers===undefined||(Array.isArray(v.challenge.resolvers)&&v.challenge.resolvers.length<=4&&v.challenge.resolvers.every(text)));
}
function resolverReceipt(v: unknown): boolean {
 return record(v) && resolverConfig(v.intent) && record(v.intent) && text(v.intent.email) && Array.isArray(v.intent.variables) && v.intent.variables.every(x=>record(x)&&text(x.name)&&member(["new","reuse"],x.mode)&&!("value" in x)) &&
 member(["admitted","backup","writing","recreating","observing","applied","rolled-back","refused","restoring","recovery-required"],v.phase) &&
 typeof v.recreateCommitted==="boolean" && (v.recreateStartedAt===undefined || timestamp(v.recreateStartedAt)) && Array.isArray(v.files) && v.files.every(x=>record(x)&&text(x.path)&&member(["static","compose","env"],x.kind)&&fileState(x.old)&&fileState(x.new)&&!("content" in x)) &&
 (v.layoutReason===undefined || member(LAYOUT_REASONS,v.layoutReason)) &&
 (v.diagnostic===undefined || (record(v.diagnostic)&&member(["compose-load-failed","compose-up-failed","static-parse-failed"],v.diagnostic.code)&&(v.diagnostic.exitCode===undefined||Number.isInteger(v.diagnostic.exitCode))&&(v.diagnostic.service===undefined||text(v.diagnostic.service))&&(v.diagnostic.line===undefined||(Number.isInteger(v.diagnostic.line)&&(v.diagnostic.line as number)>=0))&&!("detail" in v.diagnostic))) &&
 (v.replacement===undefined || (record(v.replacement)&&[v.replacement.containerId,v.replacement.name,v.replacement.startedAt,v.replacement.project,v.replacement.service,v.replacement.mountTarget,v.replacement.canonical].every(text)&&member(["project","volume","bind","single-file","unknown"],v.replacement.kind)));
}
export function decodeCertificateObservation(body: unknown): components["schemas"]["ProxyCertificateObservation"] {
 if (!record(body)||body.success!==true||!certificate(body.data)) throw new Error("proxy.invalid-response");
 return body.data as components["schemas"]["ProxyCertificateObservation"];
}
const LAYOUT_REASONS = ["unmanaged-project", "multiple-replicas", "multiple-compose-files", "compose-include", "service-extends", "compose-anchor", "env-not-regular", "compose-env-selection", "namespace-dependent", "toml-static", "entrypoint-flags", "scalar-command", "environment-static", "mixed-static-sources", "static-outside-project"];

function validStateForKind(kind:unknown,state:unknown): boolean {
 if (kind===undefined||kind==="route") return member(["pending","verified","applied-unverified","rejected","restored","recovery-required","released","not-applied"],state);
 if (kind==="resolver") return member(["pending","applied","rolled-back","refused","recovery-required","released","not-applied"],state);
 return member(["recovery-required","released"],state);
}
