// Source: docklet-next/issues/573
// Revision: b4807f4912bc07f3e51abaf552a69f2223cabd27
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original total tailnet route catalogue maps.

import type { components } from "@/api/generated/schema";
import type { CatalogueKey } from "./wire-catalogue";

type Schema = components["schemas"];

export const TAILNET_CODE_KEYS = {
  "tailnet.credential-rejected": "tailnetContract.codes.credential-rejected",
  "tailnet.coordination-unreachable": "tailnetContract.codes.coordination-unreachable",
  "tailnet.not-connected": "tailnetContract.codes.not-connected",
  "tailnet.key-mint-failed": "tailnetContract.codes.key-mint-failed",
  "tailnet.mtls-unsupported": "tailnetContract.codes.mtls-unsupported",
  "tailnet.provider-unavailable": "tailnetContract.codes.provider-unavailable",
} satisfies Record<Schema["TailnetProblem"]["code"] | NonNullable<Schema["TailnetConnection"]["code"]> | NonNullable<Schema["TailnetProvisioning"]["code"]>, CatalogueKey>;

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
