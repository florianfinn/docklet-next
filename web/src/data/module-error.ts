// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original bounded module refusal decoder shared by Direct and Edge responses.

import { AdapterOutcomeError } from "./capability";
import { MODULES } from "@/domain/modules";

export const MODULE_CODES = ["module-busy", "module-on", "operation-conflict", "feature-disabled", "module-not-found"] as const;
export type ModuleCode = (typeof MODULE_CODES)[number];
export class ModuleProblemError extends AdapterOutcomeError {
  readonly code: ModuleCode;
  readonly operation: string;
  constructor(code: ModuleCode, status: number, operation = "") {
    super({ kind: "error", code, status }); this.code = code; this.operation = operation;
  }
}

/** Hub forwarding preserves the same bounded problem body for both transports.
 * A 404 means an absent module only for module requests. */
export function moduleProblem(body: unknown, status: number, moduleRequest = true): ModuleProblemError | undefined {
  const value = body && typeof body === "object" ? body as Record<string, unknown> : {};
  if (status === 404) return moduleRequest ? new ModuleProblemError("module-not-found", status) : undefined;
  const code = value.code;
  if (typeof code !== "string" || !(MODULE_CODES as readonly string[]).includes(code)) return;
  if (code === "feature-disabled" && !MODULES.some((module) => module.id === value.feature)) return;
  const operation = typeof value.operation === "string" && value.operation.length <= 256 ? value.operation : "";
  return new ModuleProblemError(code as ModuleCode, status, operation);
}
