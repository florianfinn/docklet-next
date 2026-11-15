// Source: docklet-next/issues/473
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original total proxy wire vocabulary and bilingual presentation keys.

export const enProxyContract = {
  "states": {
    "pending": "Pending",
    "verified": "Verified",
    "applied-unverified": "Applied, not verified",
    "rejected": "Rejected",
    "restored": "Restored",
    "recovery-required": "Recovery required",
    "released": "Released",
    "not-applied": "Not applied"
  },
  "phases": {
    "admitted": "Admitted",
    "staging": "Preparing",
    "publishing": "Publishing",
    "reloading": "Reloading",
    "restarting": "Restarting",
    "observing": "Checking",
    "marking": "Checking configuration load",
    "restoring": "Restoring"
  },
  "kinds": {
    "traefik": "Traefik",
    "caddy": "Caddy"
  },
  "sources": {
    "project": "Project directory",
    "volume": "Named volume",
    "bind": "Bind mount",
    "single-file": "Single-file mount",
    "unknown": "Unknown source"
  },
  "references": {
    "localReference": "Local",
    "plugin": "Plugin",
    "internalReference": "Internal",
    "external": "External",
    "opaque": "In an unread file",
    "unresolved": "Unresolved"
  },
  "reasons": {
    "no-observable-difference": "No observable difference.",
    "not-observable": "The effect cannot be observed.",
    "container-changed": "The container changed during the operation.",
    "logs-unavailable": "Container logs are unavailable.",
    "generation-mismatch": "The agent journal belongs to another generation.",
    "operation-conflict": "The operation ID belongs to another request.",
    "module-busy": "Another operation is unfinished.",
    "stale-hash": "A configuration file changed.",
    "source-read-only": "The configuration source is read-only.",
    "unknown-operation": "The operation is unknown.",
    "operation-fenced": "The operation ID was abandoned.",
    "registration-changed": "The host registration changed.",
    "journal-lost": "The recorded journal cannot be confirmed.",
    "invalid-input": "The request is invalid.",
    "input-limit": "An input or output limit was exceeded.",
    "exec-failed": "The proxy command failed.",
    "exec-running": "The proxy command may still be running.",
    "proxy-rejected": "The proxy rejected the configuration.",
    "restore-failed": "The previous configuration could not be restored.",
    "invalid-graph": "The configuration contains invalid input.",
    "incomplete-inventory": "The proxy inventory is incomplete.",
    "opaque-input": "This input must remain unread.",
    "single-file-mount": "The configuration is mounted as a single file.",
    "refused-bind": "This bind source cannot be written.",
    "read-only-mount": "The source mount is read-only.",
    "unknown-source": "The source kind is unknown.",
    "unsupported-metadata": "The file metadata cannot be preserved safely.",
    "watch-off": "File watching is disabled.",
    "unsupported-format": "The configuration format is unsupported.",
    "template-input": "Template input cannot be edited.",
    "rule-syntax-v2": "This route uses rule syntax v2.",
    "shared-across-files": "The service is shared across files.",
    "persistence-off": "Persistent configuration is disabled.",
    "relative-import": "This relative import is unsupported.",
    "import-missing": "The route import is missing.",
    "acknowledgement-required": "Confirmation is required before writing.",
    "probe-unavailable": "The probe command is unavailable.",
    "probe-option-unavailable": "A required probe option is unavailable.",
    "admin-unavailable": "The active configuration cannot be read.",
    "agent-unavailable": "The host cannot be reached.",
    "engine-unavailable": "This proxy engine is unavailable.",
    "target-changed": "The target differs from both recorded states."
  }
};
