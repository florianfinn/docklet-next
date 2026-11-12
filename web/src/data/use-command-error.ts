// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original catalogue-backed fallback for technical command errors. M14 host maintenance and availability. M19: total contract-code catalogue maps.

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
    if (error instanceof SettingsProblemError) return settingsErrorMessage(error, all);
    if (error instanceof AdapterOutcomeError) {
      const outcome = error.outcome;
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
