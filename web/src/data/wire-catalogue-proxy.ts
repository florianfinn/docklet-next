// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original total proxy wire vocabulary and bilingual presentation keys.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";
type Schema=components["schemas"];
export const PROXY_STATE_KEYS = {
  "pending": "proxyContract.states.pending",
  "verified": "proxyContract.states.verified",
  "applied-unverified": "proxyContract.states.applied-unverified",
  "rejected": "proxyContract.states.rejected",
  "restored": "proxyContract.states.restored",
  "recovery-required": "proxyContract.states.recovery-required",
  "released": "proxyContract.states.released",
  "not-applied": "proxyContract.states.not-applied",
} satisfies Record<Schema["ProxyOperation"]["state"], CatalogueKey>;
export const PROXY_PHASE_KEYS = {
  "admitted": "proxyContract.phases.admitted",
  "staging": "proxyContract.phases.staging",
  "publishing": "proxyContract.phases.publishing",
  "reloading": "proxyContract.phases.reloading",
  "restarting": "proxyContract.phases.restarting",
  "observing": "proxyContract.phases.observing",
  "marking": "proxyContract.phases.marking",
  "restoring": "proxyContract.phases.restoring",
} satisfies Record<Schema["ProxyOperation"]["phase"], CatalogueKey>;
export const PROXY_KIND_KEYS = {
  "traefik": "proxyContract.kinds.traefik",
  "caddy": "proxyContract.kinds.caddy",
} satisfies Record<Schema["ProxyInstance"]["kind"], CatalogueKey>;
export const PROXY_SOURCE_KEYS = {
  "project": "proxyContract.sources.project",
  "volume": "proxyContract.sources.volume",
  "bind": "proxyContract.sources.bind",
  "single-file": "proxyContract.sources.single-file",
  "unknown": "proxyContract.sources.unknown",
} satisfies Record<Schema["ProxySource"]["kind"], CatalogueKey>;
export const PROXY_REFERENCE_KEYS = {
  "local": "proxyContract.references.localReference",
  "plugin": "proxyContract.references.plugin",
  "internal": "proxyContract.references.internalReference",
  "external": "proxyContract.references.external",
  "opaque": "proxyContract.references.opaque",
  "unresolved": "proxyContract.references.unresolved",
} satisfies Record<Schema["ProxyReference"]["kind"], CatalogueKey>;
export const PROXY_REASON_KEYS = {
  "no-observable-difference": "proxyContract.reasons.no-observable-difference",
  "not-observable": "proxyContract.reasons.not-observable",
  "container-changed": "proxyContract.reasons.container-changed",
  "logs-unavailable": "proxyContract.reasons.logs-unavailable",
  "generation-mismatch": "proxyContract.reasons.generation-mismatch",
  "operation-conflict": "proxyContract.reasons.operation-conflict",
  "module-busy": "proxyContract.reasons.module-busy",
  "stale-hash": "proxyContract.reasons.stale-hash",
  "source-read-only": "proxyContract.reasons.source-read-only",
  "unknown-operation": "proxyContract.reasons.unknown-operation",
  "operation-fenced": "proxyContract.reasons.operation-fenced",
  "registration-changed": "proxyContract.reasons.registration-changed",
  "journal-lost": "proxyContract.reasons.journal-lost",
  "invalid-input": "proxyContract.reasons.invalid-input",
  "input-limit": "proxyContract.reasons.input-limit",
  "exec-failed": "proxyContract.reasons.exec-failed",
  "exec-running": "proxyContract.reasons.exec-running",
  "proxy-rejected": "proxyContract.reasons.proxy-rejected",
  "restore-failed": "proxyContract.reasons.restore-failed",
  "invalid-graph": "proxyContract.reasons.invalid-graph",
  "incomplete-inventory": "proxyContract.reasons.incomplete-inventory",
  "opaque-input": "proxyContract.reasons.opaque-input",
  "single-file-mount": "proxyContract.reasons.single-file-mount",
  "refused-bind": "proxyContract.reasons.refused-bind",
  "read-only-mount": "proxyContract.reasons.read-only-mount",
  "unknown-source": "proxyContract.reasons.unknown-source",
  "unsupported-metadata": "proxyContract.reasons.unsupported-metadata",
  "watch-off": "proxyContract.reasons.watch-off",
  "unsupported-format": "proxyContract.reasons.unsupported-format",
  "template-input": "proxyContract.reasons.template-input",
  "rule-syntax-v2": "proxyContract.reasons.rule-syntax-v2",
  "shared-across-files": "proxyContract.reasons.shared-across-files",
  "persistence-off": "proxyContract.reasons.persistence-off",
  "relative-import": "proxyContract.reasons.relative-import",
  "import-missing": "proxyContract.reasons.import-missing",
  "acknowledgement-required": "proxyContract.reasons.acknowledgement-required",
  "probe-unavailable": "proxyContract.reasons.probe-unavailable",
  "probe-option-unavailable": "proxyContract.reasons.probe-option-unavailable",
  "admin-unavailable": "proxyContract.reasons.admin-unavailable",
  "agent-unavailable": "proxyContract.reasons.agent-unavailable",
  "engine-unavailable": "proxyContract.reasons.engine-unavailable",
  "target-changed": "proxyContract.reasons.target-changed",
} satisfies Record<Schema["ProxyProblem"]["code"], CatalogueKey>;
