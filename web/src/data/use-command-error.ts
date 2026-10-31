// Source: docklet-next/issues/204
// Revision: d03b817a6aa2286616ba32fc3b04a9d3f82422ed
// Retrieved: 2026-10-24
// License: Apache-2.0
// Deviations: Original catalogue-backed fallback for technical command errors.

import { operationReasonKey } from "./operation-reason";
import { useCallback } from "react";
import { useTranslations } from "use-intl";
import { AdapterOutcomeError } from "./capability";

/** Exception messages and unknown server codes have no UI translation contract. */
export function useCommandError() {
  const operation = useTranslations("containers.operationReasons");
  const t = useTranslations("common.commandErrors");
  const capability = useTranslations("activity.capability");
  return useCallback((error: unknown): string => {
    if (error instanceof AdapterOutcomeError) {
      const outcome = error.outcome;
      const key = operationReasonKey(outcome.kind === "not-available" ? outcome.reasonCode : outcome.code);
      if (key) return operation(key);
      if (outcome.kind === "not-available") return capability(outcome.reasonCode === "capability.mapping-unavailable" ? "mappingUnavailable" : "unavailable");
      if (outcome.kind === "error") {
        switch (outcome.code) {
          case "agent.unavailable": return t("unreachable");
          case "agent.invalid-token": return t("credentialRejected");
          case "agent.update-required": return t("updateRequired");
        }
        if (outcome.status === 401) return t("signIn");
        if (outcome.status === 403) return t("denied");
      }
    }
    return t("failed");
  }, [t, capability, operation]);
}
