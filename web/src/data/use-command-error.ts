// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original catalogue-backed fallback for technical command errors. M14 host maintenance and availability. M19: total contract-code catalogue maps; optional-module foundation (#472).

import { MODULE_CODE_KEYS } from "./wire-catalogue-modules";
import { ModuleProblemError } from "./module-error";
import { operationReasonKey } from "./operation-reason";
import { useCallback } from "react";
import { useTranslations } from "use-intl";
import { REQUEST_CODE_KEYS, CONTAINER_CODE_KEYS, HOST_CODE_KEYS, ACCESS_CODE_KEYS, WIDGET_CODE_KEYS } from "./wire-catalogue-errors";
import { wireKey } from "./wire-catalogue";
import { isWidgetErrorCode } from "@/domain/service-widgets";
import { AdapterOutcomeError } from "./capability";
import { SettingsProblemError, settingsErrorMessage } from "./settings-error";

/** Exception messages and unknown server codes have no UI translation contract. */
export function useCommandError() {
  const operation = useTranslations("containers.operationReasons");
  const t = useTranslations("common.commandErrors");
  const capability = useTranslations("activity.capability");
  const all = useTranslations();
  return useCallback((error: unknown): string => {
    // Typed settings refusals (for example an invalid time zone) carry their own catalogue keys.
    if (error instanceof ModuleProblemError) return all(wireKey(MODULE_CODE_KEYS, error.code) ?? "settings.modules.codes.module-not-found", { operation: error.operation });
 if (error instanceof SettingsProblemError) return settingsErrorMessage(error, all);
    if (error instanceof AdapterOutcomeError) {
      const outcome = error.outcome;
 if (outcome.kind === "error" && outcome.code === "runtime.invalid-acknowledgement") return all("common.runtimePeers.invalid");
      if (outcome.kind === "error" && outcome.code === "runtime.target-unavailable" && outcome.blocker) {
        const blocker = outcome.blocker;
 if (blocker.reason === "compose.duplicate" || blocker.reason === "compose.env-file-missing") return all("common.runtimePeers.duplicate", { name: blocker.name });
        const reason = all(blocker.reason === "compose.selection-unsupported" ? "images.errors.runtime_blocker-selection" : "images.errors.runtime_blocker-identity");
        return all(blocker.archived ? "images.errors.runtime_archived-stack-blocker" : "images.errors.runtime_stack-blocker", { name: blocker.name, reason });
      }
      const key = operationReasonKey(outcome.kind === "not-available" ? outcome.reasonCode : outcome.code);
      if (key && !["agent.unavailable", "agent.invalid-token", "agent.update-required"].includes(outcome.kind === "not-available" ? outcome.reasonCode : outcome.code)) return operation(key);
      if (outcome.kind === "not-available") return capability(outcome.reasonCode === "capability.mapping-unavailable" ? "mappingUnavailable" : "unavailable");
      if (outcome.kind === "error") {
        if (isWidgetErrorCode(outcome.code)) return all(WIDGET_CODE_KEYS[outcome.code]);
        switch (outcome.code) {
          case "agent.unavailable": return t("unreachable");
          case "agent.invalid-token": return t("credentialRejected");
          case "agent.update-required": return t("updateRequired");
        }
        const contractKey = wireKey(REQUEST_CODE_KEYS, outcome.code) ?? wireKey(CONTAINER_CODE_KEYS, outcome.code) ?? wireKey(HOST_CODE_KEYS, outcome.code) ?? wireKey(ACCESS_CODE_KEYS, outcome.code);
        if (contractKey) return all(contractKey);
        if (outcome.status === 401) return t("signIn");
        if (outcome.status === 403) return t("denied");
      }
    }
    return t("failed");
  }, [t, capability, operation, all]);
}
