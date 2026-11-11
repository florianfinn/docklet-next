// Source: docklet-next/issues/391
// Revision: 6b89ee3606392c8b65c6dbd6577ffe0773e581b1
// Retrieved: 2026-11-11
// License: Apache-2.0
// Deviations: Original app query client defaults, moved out of the provider; M23 adds the maintenance retry policy.

import { QueryClient } from "@tanstack/react-query";
import { maintenanceRetry, maintenanceRetryDelay } from "./maintenance-retry";

/** The app's query defaults: no retries except reads refused by hub maintenance. */
export function createAppQueryClient(): QueryClient {
  return new QueryClient({
    defaultOptions: {
      queries: { retry: maintenanceRetry, retryDelay: maintenanceRetryDelay, refetchOnWindowFocus: false, staleTime: 30_000 },
      mutations: { retry: false },
    },
  });
}
