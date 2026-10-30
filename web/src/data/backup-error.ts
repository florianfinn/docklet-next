// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original safe backup problem presentation.

import type { useTranslations } from "use-intl";
import type { components } from "@/api/generated/schema";
import { catalogs } from "@/app/i18n/catalog";
import { AdapterOutcomeError } from "./capability";

export class BackupError extends AdapterOutcomeError {
  readonly problem: Pick<
    components["schemas"]["BackupProblem"],
    "code" | "activityId" | "reason" | "hosts" | "volumes"
  >;
  constructor(
    problem: Pick<components["schemas"]["BackupProblem"], "code" | "activityId" | "reason" | "hosts" | "volumes">,
    status?: number,
  ) {
    super({ kind: "error", code: problem.code, status });
    this.problem = problem;
  }
}

export function backupCodeKey(
  code?: string,
): `volumes.backupCodes.${keyof (typeof catalogs)["en-US"]["volumes"]["backupCodes"]}` | undefined {
  const key = code?.replaceAll(".", "_");
  return key && Object.hasOwn(catalogs["en-US"].volumes.backupCodes, key)
    ? `volumes.backupCodes.${key as keyof (typeof catalogs)["en-US"]["volumes"]["backupCodes"]}`
    : undefined;
}

const ACCESS_KEYS = {
  "agent.unavailable": "common.commandErrors.unreachable",
  "agent.invalid-token": "common.commandErrors.credentialRejected",
  "agent.update-required": "common.commandErrors.updateRequired",
} as const;

/** Server detail/title/error are deliberately never carried into user feedback. */
export function backupErrorMessage(error: unknown, translate: ReturnType<typeof useTranslations>): string {
  const code = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
  const key = backupCodeKey(code) ?? (code === "volume.outcome-unknown" ? "volumes.errors.outcome-unknown" : code && code in ACCESS_KEYS ? ACCESS_KEYS[code as keyof typeof ACCESS_KEYS] : undefined);
  const reason = error instanceof BackupError ? error.problem.reason : undefined;
  return [
    translate(key ?? "common.commandErrors.failed"),
    reason && Object.hasOwn(catalogs["en-US"].volumes.uploadReasons, reason)
      ? translate(`volumes.uploadReasons.${reason}`)
      : undefined,
  ]
    .filter(Boolean)
    .join(" ");
}
