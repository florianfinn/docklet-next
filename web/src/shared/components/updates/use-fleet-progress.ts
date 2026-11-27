// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { useSyncExternalStore } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { useCurrentUser } from "@/data/queries";

export function useFleetProgress() {
  const adapter = useAdapter(), lifetime = identityLifetime(adapter), user = useCurrentUser();
  const version = useSyncExternalStore(lifetime.listen, lifetime.version);
  const enabled = lifetime.active() && user.data?.isAdmin === true;
  const query = useQuery({ queryKey: ["fleet-update", version], queryFn: () => adapter.getFleetUpdate(), enabled,
    retry: false, refetchInterval: q => q.state.data?.active ? 3000 : false });
  return { ...query, data: enabled ? query.data : undefined, allowed: enabled, version };
}
