// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original selection and viewer lifetime for advisory detail reads. M21 #437: advisory detail.

import { useEffect, useReducer, useRef, useState, useSyncExternalStore } from "react";
import { isCancelledError } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import type { AdvisoryRead } from "@/data/vulnerability-advisory";

export function useAdvisoryDetail(key: string | null, hostIds?: readonly string[]) {
  const adapter = useAdapter(), lifetime = identityLifetime(adapter);
  const identity = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const active = lifetime.active();
  const [revision, reload] = useReducer((v: number) => v + 1, 0);
  const scope = JSON.stringify(hostIds ?? []);
  const owner = JSON.stringify([key, scope, identity, revision, active]);
  const generation = useRef(0);
  const [state, setState] = useState<{ owner: string; data?: AdvisoryRead; error?: unknown }>();
  useEffect(() => adapter.subscribeChanges((topics) => { if (topics.includes("security") || topics.includes("hosts")) reload(); }), [adapter]);
  useEffect(() => {
    const sequence = ++generation.current, controller = new AbortController();
    if (key && active) {
      void adapter.getVulnerabilityAdvisory(key, JSON.parse(scope) as string[], controller.signal).then(
        (data) => { if (sequence === generation.current && !controller.signal.aborted && lifetime.version() === identity && lifetime.active()) setState({ owner, data }); },
        (error: unknown) => { if (sequence === generation.current && !controller.signal.aborted && !isCancelledError(error)) setState({ owner, error }); },
      );
    }
    return () => { controller.abort(); };
  }, [adapter, key, scope, owner, active, lifetime, identity]);
  return { data: active && state?.owner === owner ? state.data : undefined, error: active && state?.owner === owner ? state.error : undefined, reload };
}
