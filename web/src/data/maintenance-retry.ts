// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original query retry policy for reads refused by hub maintenance.

import { AdapterOutcomeError } from "./capability";
import { MAINTENANCE_CODE } from "./http/problem-code";

/** Every 4 s, up to 30 times: together the 120 s bound of a maintenance hold. */
export const MAINTENANCE_RETRY_DELAY_MS = 4_000;
export const MAINTENANCE_RETRIES = 30;

/** A maintenance refusal in any reader's error shape: an adapter outcome or a reader failure carrying the code. */
export function isMaintenanceError(error: unknown): boolean {
  if (error instanceof AdapterOutcomeError) return error.outcome.kind === "error" && error.outcome.code === MAINTENANCE_CODE;
  return !!error && typeof error === "object" && (error as { code?: unknown }).code === MAINTENANCE_CODE;
}

/** Only maintenance refusals retry: TanStack keeps a query's cached data and success state while it retries, so no
 * consumer replaces its content with an error during maintenance. Every other failure surfaces at once. */
export const maintenanceRetry = (failureCount: number, error: unknown): boolean => isMaintenanceError(error) && failureCount < MAINTENANCE_RETRIES;
export const maintenanceRetryDelay = (): number => MAINTENANCE_RETRY_DELAY_MS;
