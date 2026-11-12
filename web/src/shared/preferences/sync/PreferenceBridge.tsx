// Source: docklet-next/docs/design/preferences.md
// Revision: 14e11ba9c022d57c29bd36c8ed24780763327d0c
// Retrieved: 2026-11-03
// License: Apache-2.0
// Deviations: Original mount of the profile preference sync for issue #339.

import { useEffect, useRef } from "react";

import { usePrincipal } from "@/data/use-principal";
import { useAdapter } from "@/data/use-adapter";

import { createPreferenceSync, type PreferenceSync } from "./engine";
import { SECTION_BINDINGS } from "./sections";

/** Keeps the browser stores in step with the signed-in user's profile document. */
export function PreferenceBridge() {
  const adapter = useAdapter();
  const principal = usePrincipal().data;
  const identity = adapter.kind === "disconnected" ? null : principal?.id ?? (adapter.kind === "demo" ? "demo-admin" : null);
  const sync = useRef<PreferenceSync | null>(null);
  useEffect(() => {
    const s = createPreferenceSync({ api: adapter, bindings: SECTION_BINDINGS, storage: typeof localStorage === "undefined" ? undefined : localStorage });
    sync.current = s;
    return () => {
      s.dispose();
      sync.current = null;
    };
  }, [adapter]);
  useEffect(() => sync.current?.setIdentity(identity), [adapter, identity]);
  return null;
}
