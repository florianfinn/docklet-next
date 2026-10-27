// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { useEffect, useRef, useState } from "react";
import { useQueryClient } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { ScanAdmissionError } from "@/domain/security";

export function useScanAction() {
  const adapter = useAdapter(), client = useQueryClient(), t = useTranslations("security");
  const owner = useRef({ generation: 0 }), [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  useEffect(() => {
    const state = owner.current;
    const off = identityLifetime(adapter).listen(() => { ++state.generation; setBusy(false); setError(undefined); });
    return () => { ++state.generation; off(); };
  }, [adapter]);
  async function execute(command: () => Promise<unknown>) {
    const version = owner.current.generation; setBusy(true); setError(undefined);
    try { await command(); }
    catch (e) { if (version === owner.current.generation) setError(e instanceof ScanAdmissionError ? t("scanFailed") : t("scanUnavailable")); }
    finally { if (version === owner.current.generation) { setBusy(false); void client.invalidateQueries({ queryKey: ["actions"] }); void client.invalidateQueries({ queryKey: ["security"] }); } }
  }
  return {
    busy, error,
    start: (input?: { hostIds?: readonly string[]; imageId?: string }) => execute(() => adapter.runScan(input)),
    enable: (input?: { hostIds?: readonly string[] }, scan = false) => execute(async () => {
      await adapter.setScanEnabled(true);
      if (scan) await adapter.runScan(input);
    }),
  };
}
