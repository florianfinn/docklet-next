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
const states = ["pending", "verified", "applied-unverified", "rejected", "restored", "recovery-required", "released", "not-applied"] as const satisfies readonly ProxyOperation["state"][];
const phases = ["admitted", "staging", "publishing", "reloading", "restarting", "observing", "marking", "restoring"] as const satisfies readonly ProxyOperation["phase"][];
const reasons = ["no-observable-difference", "not-observable", "container-changed", "logs-unavailable", "generation-mismatch", "operation-conflict", "module-busy", "stale-hash", "source-read-only", "unknown-operation", "operation-fenced", "registration-changed", "journal-lost", "invalid-input", "input-limit", "exec-failed", "exec-running", "proxy-rejected", "restore-failed", "invalid-graph", "incomplete-inventory", "opaque-input", "single-file-mount", "refused-bind", "read-only-mount", "unknown-source", "unsupported-metadata", "watch-off", "unsupported-format", "template-input", "rule-syntax-v2", "shared-across-files", "persistence-off", "relative-import", "import-missing", "acknowledgement-required", "probe-unavailable", "probe-option-unavailable", "admin-unavailable", "agent-unavailable", "engine-unavailable", "target-changed"] as const satisfies readonly ProxyReason[];
const record = (v: unknown): v is Record<string, unknown> => !!v && typeof v === "object" && !Array.isArray(v);
const member = (items: readonly string[], v: unknown) => typeof v === "string" && items.includes(v);
// Diagnostic is optional engine text; its bounds come from the wire contract.
function diagnostic(v: unknown): boolean { return record(v) && (v.path === undefined || (typeof v.path === "string" && v.path.length <= 1024)) && (v.line === undefined || (Number.isInteger(v.line) && (v.line as number) >= 0)) && (v.detail === undefined || (typeof v.detail === "string" && v.detail.length <= 4096)); }
function fileState(v: unknown): boolean { return record(v) && typeof v.present === "boolean" && typeof v.hash === "string" && (v.present ? /^[a-f0-9]{64}$/.test(v.hash) : v.hash === ""); }
export function decodeProxyOperation(body: unknown): ProxyOperation {
  if (!record(body) || body.success !== true || !record(body.data)) throw new Error("proxy.invalid-response");
  const v = body.data;
  if (!member(states, v.state) || !member(phases, v.phase) || (v.reason !== undefined && v.reason !== "" && !member(reasons, v.reason)) || (v.evidence !== undefined && !member(["routing", "configuration", "load"], v.evidence))) throw new Error("proxy.invalid-response");
  if (!["id", "instanceId", "generation", "registration", "digest", "target", "createdAt"].every((k) => typeof v[k] === "string" && v[k] !== "") || !fileState(v.old) || !fileState(v.new) || (v.current !== undefined && !fileState(v.current)) || (v.diagnostic !== undefined && !diagnostic(v.diagnostic))) throw new Error("proxy.invalid-response");
  return v as ProxyOperation;
}
// Presentation retains the exact recorded outcome; only verified is success.
export function proxyOutcome(operation: ProxyOperation): { state: ProxyOperation["state"]; tone: "success" | "warning" | "danger" | "neutral"; reason: string | undefined;labelKey:CatalogueKey;reasonKey:CatalogueKey|undefined } {
  const state = operation.state;
  const tone = state === "verified" ? "success" : state === "recovery-required" || state === "rejected" ? "danger" : state === "applied-unverified" || state === "restored" ? "warning" : "neutral";
  return { state, tone, reason: operation.reason,labelKey:PROXY_STATE_KEYS[state],reasonKey:wireKey(PROXY_REASON_KEYS,operation.reason) };
}
