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
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { ScanAdmissionError, type ScanAdmission } from "@/domain/security";

export function useScanAction() {
  const adapter = useAdapter(), client = useQueryClient(), t = useTranslations("security");
  const hosts = useHostScope();
  const [refusals, setRefusals] = useState<ScanAdmission[]>([]);
  const owner = useRef({ generation: 0 }), [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  useEffect(() => {
    const state = owner.current;
    const off = identityLifetime(adapter).listen(() => { ++state.generation; setBusy(false); setError(undefined); setRefusals([]); });
    return () => { ++state.generation; off(); };
  }, [adapter]);
  async function execute(command: () => Promise<unknown>) {
    const version = owner.current.generation; setBusy(true); setError(undefined); setRefusals([]);
    try { await command(); }
    catch (e) {
      if (version === owner.current.generation) {
        if (e instanceof ScanAdmissionError && e.admissions.length) setRefusals(e.admissions.filter((a) => !a.activityId));
        else if (e instanceof ScanAdmissionError) setError(t("scanFailed"));
        else setError(t("scanUnavailable"));
      }
    }
    finally { if (version === owner.current.generation) { setBusy(false); void client.invalidateQueries({ queryKey: ["actions"] }); void client.invalidateQueries({ queryKey: ["security"] }); } }
  }
  const skipped = (a: ScanAdmission) => ["vulnerability.host-offline", "vulnerability.host-pending"].includes(a.code ?? "") || ["offline", "pending"].includes(hosts.byId.get(a.hostId)?.status ?? "");
  const describe = (rows: ScanAdmission[]) => rows.map((a) => t("hostRefused", { host: hosts.byId.get(a.hostId)?.name ?? a.hostId, code: a.code ?? "vulnerability.dispatch-unknown" })).join("; ");
  const failed = refusals.filter((a) => !skipped(a)), notes = refusals.filter(skipped);
  return {
    busy, error: error ?? (failed.length ? describe(failed) : undefined), note: notes.length ? describe(notes) : undefined,
    start: (input?: { hostIds?: readonly string[]; imageId?: string }) => execute(() => adapter.runScan(input)),
    enable: (input?: { hostIds?: readonly string[] }, scan = false) => execute(async () => {
      await adapter.setScanEnabled(true);
      if (scan) await adapter.runScan(input);
    }),
  };
}
