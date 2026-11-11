// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original query retry policy for reads refused by hub maintenance.

import { AdapterOutcomeError } from "./capability";

/** Every 4 s, up to 30 times: together the 120 s bound of a maintenance hold. */
export const MAINTENANCE_RETRY_DELAY_MS = 4_000;
export const MAINTENANCE_RETRIES = 30;

export function isMaintenanceError(error: unknown): boolean {
  return error instanceof AdapterOutcomeError && error.outcome.kind === "error" && error.outcome.code === "backup.maintenance";
}

/** Only maintenance refusals retry: TanStack keeps a query's cached data and success state while it retries, so no
 * consumer replaces its content with an error during maintenance. Every other failure surfaces at once. */
export const maintenanceRetry = (failureCount: number, error: unknown): boolean => isMaintenanceError(error) && failureCount < MAINTENANCE_RETRIES;
export const maintenanceRetryDelay = (): number => MAINTENANCE_RETRY_DELAY_MS;
