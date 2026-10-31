// Source: docklet-next/issues/297
// Revision: 97a16304639854dc5aca368c879689d7a2161671
// Retrieved: 2026-10-29
// License: Apache-2.0
// Deviations: Original typed notification and Auto Heal problem presentation. M14 host maintenance and availability.

import type { useTranslations } from "use-intl";
import { AdapterOutcomeError } from "./capability";

/** Producer codes the settings UI translates; anything else stays generic. */
export const SETTINGS_PROBLEM_CODES = [
  "notification.config-invalid", "notification.hub-only", "notification.test-busy",
  "autoheal.settings-invalid", "autoheal.settings-env-managed", "settings.key-managed", "settings.ui-configuration-disabled",
] as const;
const ACCESS_CODES = ["agent.unavailable", "agent.invalid-token", "agent.update-required", "agent.ui-configuration-disabled"] as const;
export type SettingsProblemCode = (typeof SETTINGS_PROBLEM_CODES)[number] | (typeof ACCESS_CODES)[number];

/** A typed refusal; `field` is the producer's JSON pointer, never a value. */
export class SettingsProblemError extends AdapterOutcomeError {
  readonly code: SettingsProblemCode;
  readonly field?: string;
  constructor(code: SettingsProblemCode, status: number, field?: string) {
    super({ kind: "error", code, status });
    this.code = code;
    this.field = field;
  }
}

/** Maps a producer body to a typed refusal, else to a status-only error without server text. */
export function settingsProblem(body: unknown, status: number): AdapterOutcomeError {
  const value = body && typeof body === "object" ? (body as { code?: unknown; field?: unknown }) : {};
  const code = value.code;
  const field = typeof value.field === "string" && value.field.length <= 256 ? value.field : undefined;
  if (typeof code === "string" && ([...SETTINGS_PROBLEM_CODES, ...ACCESS_CODES] as string[]).includes(code))
    return new SettingsProblemError(code as SettingsProblemCode, status, field);
  return new AdapterOutcomeError({ kind: "error", code: "http.error", status });
}

const KEYS: Record<SettingsProblemCode, string> = {
  "notification.config-invalid": "settings.codes.notification.config-invalid",
  "notification.hub-only": "settings.codes.notification.hub-only",
  "notification.test-busy": "settings.codes.notification.test-busy",
  "autoheal.settings-invalid": "settings.codes.autoheal.settings-invalid",
  "autoheal.settings-env-managed": "settings.codes.autoheal.settings-env-managed",
  "settings.ui-configuration-disabled": "hosts.errors.uiConfigurationDisabled",
  "agent.ui-configuration-disabled": "hosts.errors.uiConfigurationDisabled",
  "settings.key-managed": "settings.codes.settings.key-managed",
  "agent.unavailable": "common.commandErrors.unreachable",
  "agent.invalid-token": "common.commandErrors.credentialRejected",
  "agent.update-required": "common.commandErrors.updateRequired",
};

/** Server detail and title never reach user feedback; a refused write reads as a refusal. */
export function settingsErrorMessage(error: unknown, translate: ReturnType<typeof useTranslations>): string {
  if (error instanceof SettingsProblemError) return translate(KEYS[error.code] as never);
  if (error instanceof AdapterOutcomeError && error.outcome.kind === "not-available") return translate("activity.capability.unavailable");
  const status = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.status : undefined;
  if (status === 401) return translate("common.commandErrors.signIn");
  if (status === 403) return translate("common.commandErrors.denied");
  return translate("common.commandErrors.failed");
}
