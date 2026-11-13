// Source: docklet-next/issues/370
// Revision: e83119924556ce53fea232ceac7a270bd595304b
// Retrieved: 2026-11-13
// License: Apache-2.0
// Deviations: Original latest agent restart read for the host panel after a reload.

import { useQuery } from "@tanstack/react-query";
import { useSyncExternalStore } from "react";
import { useLocale } from "use-intl";

import type { ActionRun } from "@/domain/model";

import { identityLifetime } from "./identity-lifetime";
import { renderRun } from "./message-code";
import { useAdapter } from "./use-adapter";

/**
 * The host's active or newest agent restart, read once per panel mount and identity. The query is
 * dropped with the panel, so a reopen reads again; an active run continues as an in-session run.
 */
export function useLatestAgentRestart(hostId: string): ActionRun | undefined {
  const adapter = useAdapter();
  const locale = useLocale();
  const lifetime = identityLifetime(adapter);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const read = adapter.latestAgentRestart;
  const query = useQuery({
    queryKey: ["agent-restart-latest", hostId, generation],
    queryFn: async ({ signal }) => (await read?.(hostId, signal)) ?? null,
    enabled: read !== undefined && hostId !== "0",
    staleTime: Number.POSITIVE_INFINITY, gcTime: 0, retry: false,
  });
  return query.data ? renderRun(query.data, locale) : undefined;
}
