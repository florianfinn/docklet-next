// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original total tailnet route catalogue maps.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const TAILNET_CODE_KEYS = {
  "tailnet.mint-refused": "connections.mint-refused",
  "tailnet.test-cleanup-failed": "connections.test-cleanup-failed",
  "tailnet.test-running": "connections.test-running",
  "tailnet.credential-rejected": "tailnetContract.codes.credential-rejected",
  "tailnet.coordination-unreachable": "tailnetContract.codes.coordination-unreachable",
  "tailnet.not-connected": "tailnetContract.codes.not-connected",
  "tailnet.key-mint-failed": "tailnetContract.codes.key-mint-failed",
  "tailnet.mtls-unsupported": "tailnetContract.codes.mtls-unsupported",
  "tailnet.provider-unavailable": "tailnetContract.codes.provider-unavailable",
} satisfies Record<Schema["TailnetProblem"]["code"] | NonNullable<Schema["TailnetConnection"]["code"]> | NonNullable<Schema["TailnetProvisioning"]["code"]> | NonNullable<Schema["TailnetTestCheck"]["code"]>, CatalogueKey>;

export const TAILNET_CONNECTION_KEYS = {
  "off": "tailnetContract.connection.off",
  "joining": "tailnetContract.connection.joining",
  "connected": "tailnetContract.connection.connected",
  "control-unreachable": "tailnetContract.connection.control-unreachable",
  "error": "tailnetContract.connection.error",
} satisfies Record<Schema["TailnetConnection"]["state"], CatalogueKey>;

export const TAILNET_PROVISIONING_KEYS = {
  "not-configured": "tailnetContract.provisioning.not-configured",
  "not-applicable": "tailnetContract.provisioning.not-applicable",
  "checking": "tailnetContract.provisioning.checking",
  "ok": "tailnetContract.provisioning.ok",
  "rejected": "tailnetContract.provisioning.rejected",
  "unavailable": "tailnetContract.provisioning.unavailable",
} satisfies Record<Schema["TailnetProvisioning"]["state"], CatalogueKey>;

export const TAILNET_PEER_KEYS = {
  "online": "tailnetContract.peer.online",
  "offline": "tailnetContract.peer.offline",
  "unknown": "tailnetContract.peer.unknown",
} satisfies Record<Schema["TailnetHostPeer"]["state"], CatalogueKey>;

export const TAILNET_TEST_STATE_KEYS = {
  "pass": "connections.steps.testStates.pass",
  "fail": "connections.steps.testStates.fail",
  "skipped": "connections.steps.testStates.skipped",
  "unknown": "connections.steps.testStates.unknown",
} satisfies Record<Schema["TailnetTestCheck"]["state"], CatalogueKey>;

export const TAILNET_TEST_CHECK_KEYS = {
  "credential": "connections.steps.testChecks.credential",
  "agent-key": "connections.steps.testChecks.agent-key",
  "cleanup": "connections.steps.testChecks.cleanup",
  "hub-joined": "connections.steps.testChecks.hub-joined",
  "hub-tag": "connections.steps.testChecks.hub-tag",
  "hub-address": "connections.steps.testChecks.hub-address",
} satisfies Record<Schema["TailnetTestCheck"]["id"], CatalogueKey>;
