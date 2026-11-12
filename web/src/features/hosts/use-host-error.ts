// Source: docklet-next/issues/98
// Revision: fdb37e836f708bd9ff76f75075bbb3e60ae2f0b3
// Retrieved: 2026-10-18
// License: Apache-2.0
// Deviations: Original host outcome localization. M14 host maintenance and availability.

import { useTranslations } from "use-intl";

import { operationReasonKey } from "@/data/operation-reason";
import { errorCode } from "@/data/api";
import { AdapterOutcomeError } from "@/data/capability";

/** Technical outcomes never become user-facing host error text. */
export function useHostError() {
  const operation = useTranslations("containers.operationReasons");
  const t = useTranslations("hosts.errors");
  const capability = useTranslations("activity.capability");
  return (error: unknown): string => {
    if (error instanceof AdapterOutcomeError) {
      const outcome = error.outcome;
      const specific = operationReasonKey(outcome.kind === "not-available" ? outcome.reasonCode : outcome.code);
      if (specific && !["agent.unavailable", "agent.invalid-token", "agent.update-required", "agent.ui-configuration-disabled", "host.unreachable"].includes(outcome.kind === "not-available" ? outcome.reasonCode : outcome.code)) return operation(specific);
      if (outcome.kind === "not-available") return capability(outcome.reasonCode === "capability.mapping-unavailable" ? "mappingUnavailable" : "unavailable");
      if (outcome.kind === "conflict") return t("busy");
      switch (outcome.code) {
        case "host.name-invalid": return t("invalid-name");
        case "host.name-taken": return t("exists");
        case "host.unreachable":
        case "agent.unavailable": return t("unreachable");
        case "agent.invalid-token": return t("credentialRejected");
        case "agent.update-required": return t("updateRequired");
        case "agent.ui-configuration-disabled": return t("uiConfigurationDisabled");
      }
    }
    const code = errorCode(error);
    switch (code) {
      case "exists": case "not-found": case "host-in-use": case "invalid-name": case "invalid-color": return t(code);
      case "host-offline": return t("unreachable");
      default: return t("failed");
    }
  };
}
