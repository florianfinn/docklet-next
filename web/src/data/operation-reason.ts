// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original shared presentation keys for M14 operation refusals. M14 host maintenance and availability; M15 simple-user rights and identity contracts. M19: total contract-code catalogue maps.

import { scopedKey, wireKey, type CatalogueKey } from "./wire-catalogue";
import { ACCESS_CODE_KEYS } from "./wire-catalogue-errors";

const CREDENTIAL_REJECTED_KEY = "containers.operationReasons.credentialRejected" satisfies CatalogueKey;

const OPERATION_REASON_KEYS = {
  ...ACCESS_CODE_KEYS,
  "container.origin-locked": "containers.operationReasons.originLocked",
  "container.recreate-recovered": "containers.operationReasons.recreateRecovered",
  "container.recreate-ambiguous": "containers.operationReasons.recreateAmbiguous",
  "runtime.idempotency-mismatch": "containers.operationReasons.idempotencyMismatch",
  "host.unreachable": "containers.operationReasons.hostUnavailable",
  "agent.unavailable": "containers.operationReasons.hostUnavailable",
  "agent.invalid-token": CREDENTIAL_REJECTED_KEY,
  "agent.update-required": "containers.operationReasons.updateRequired",
  "agent.ui-configuration-disabled": "containers.operationReasons.uiConfigurationDisabled",
  "settings.ui-configuration-disabled": "containers.operationReasons.uiConfigurationDisabled",
  "agent.restart-superseded": "containers.operationReasons.restartSuperseded",
  "agent.restart-timeout": "containers.operationReasons.restartTimeout",
  "agent.restart-unsupported": "containers.operationReasons.restartUnsupported",
  "agent.restart-interrupted": "containers.operationReasons.restartInterrupted",
  "agent.restart-unconfirmed": "containers.operationReasons.restartUnconfirmed",
  "host.busy": "containers.operationReasons.hostBusy",
  "host.restarting": "containers.operationReasons.hostRestarting",
} satisfies Record<string, CatalogueKey>;

export function operationReasonKey(code: string) {
  const key = wireKey(OPERATION_REASON_KEYS, code);
  return key ? scopedKey(key, "containers.operationReasons.") : undefined;
}
