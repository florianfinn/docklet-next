// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original adapter session subscription and tab recovery bridge.

import { useEffect } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { useCurrentUser } from "@/data/queries";
import { identityLifetime } from "@/data/identity-lifetime";
import { useHubUpdate } from "./use-hub-update";

export function HubUpdateRecovery() {
  const adapter = useAdapter(), client = useQueryClient();
  const user = useCurrentUser(adapter.kind === "http");
  const { state } = useHubUpdate();
  const id = user.data?.id, admin = user.data?.isAdmin;
  useEffect(() => {
    if (adapter.kind !== "http" || admin !== true || !id) return;
    const recover = () => { if (identityLifetime(adapter).active()) void adapter.hubUpdateSession?.recover(); };
    recover();
    return identityLifetime(adapter).listen(recover);
  }, [adapter, id, admin]);
  useEffect(() => {
    if (state.phase === "done" || state.phase === "failed" || state.phase === "unknown") {
      void client.invalidateQueries({ queryKey: ["hub", "version-hint"] });
      void client.invalidateQueries({ queryKey: ["settings", "self-update", "0"] });
    }
  }, [state.phase, client]);
  return null;
}
