// Source: docklet-next/issues/284
// Revision: 7ccefa10105544af6787161faaf8328e4a44b45f
// Retrieved: 2026-10-28
// License: Apache-2.0
// Deviations: Original safe backup problem presentation. M19: total contract-code catalogue maps. M20 #360: web hardening step (#359 #24 #175 #177 #383 #421 #271).

import type { useTranslations } from "use-intl";
import type { components } from "@/api/generated/schema";
import { wireKey, type CatalogueKey } from "./wire-catalogue";
import { BACKUP_CODE_KEYS, BACKUP_REASON_KEYS } from "./wire-catalogue-backup";
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

// A hub audit event also occurs in historical backup rows; preserve its backup-specific wording.
const BACKUP_AUDIT_KEYS = {
  "volume.backup.target.key_viewed": "volumes.backupCodes.volume_backup_target_key_viewed",
} satisfies Record<string, CatalogueKey>;

export function backupCodeKey(
  code?: string,
) {
  return wireKey(BACKUP_CODE_KEYS, code) ?? wireKey(BACKUP_AUDIT_KEYS, code);
}

const ACCESS_KEYS = {
  "agent.unavailable": "common.commandErrors.unreachable",
  "agent.invalid-token": "common.commandErrors.credentialRejected",
  "agent.update-required": "common.commandErrors.updateRequired",
} as const;

/** Server detail/title/error are deliberately never carried into user feedback. */
export function backupErrorMessage(error: unknown, translate: ReturnType<typeof useTranslations<never>>): string {
  const code = error instanceof AdapterOutcomeError && error.outcome.kind === "error" ? error.outcome.code : undefined;
  const key = backupCodeKey(code) ?? (code === "volume.outcome-unknown" ? "volumes.errors.outcome-unknown" : code && code in ACCESS_KEYS ? ACCESS_KEYS[code as keyof typeof ACCESS_KEYS] : undefined);
  const reason = error instanceof BackupError ? wireKey(BACKUP_REASON_KEYS, error.problem.reason) : undefined;
  return [
    translate(key ?? "common.commandErrors.failed"),
    reason ? translate(reason) : undefined,
  ]
    .filter(Boolean)
    .join(" ");
}
