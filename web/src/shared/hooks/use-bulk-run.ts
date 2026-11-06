import { bulkResultStore } from "@/shared/components/bulk-result-store";
import { identityLifetime } from "@/data/identity-lifetime";
// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded bulk admission and captured result ownership. M19 web consumer integration.

import { useState } from "react";
import { useTranslations } from "use-intl";
import { toast } from "sonner";
import { useQueries } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { bulkOperation, runBulk, type BulkAction, type BulkItem, type BulkResult } from "./bulk-run";

export function useBulkCapabilities(items: readonly BulkItem[], action: BulkAction) {
  const adapter = useAdapter(), t = useTranslations("common.bulkRun");
  const results = useQueries({ queries: items.map((item) => ({ queryKey: ["capability", bulkOperation(item.kind, action), item.kind, item.id], queryFn: () => adapter.getCapability(bulkOperation(item.kind, action), { kind: item.kind, id: item.id }) })) });
  const allowed = items.filter((item, index) => !item.stale && results[index]?.data?.available === true).length;
  return { available: allowed > 0, reason: t("skipCount", { count: items.length - allowed }) };
}
export function useBulkRun() {
  const adapter = useAdapter(), t = useTranslations("common.bulkRun");
  const [running, setRunning] = useState(false);
  const run = async (items: readonly BulkItem[], action: BulkAction, revisions?: ReadonlyMap<string, string>, skipped: BulkResult[] = []) => {
    const lifetime = identityLifetime(adapter), version = lifetime.version();
    setRunning(true);
    try {
      const values = [...await runBulk(adapter, items, action, revisions), ...skipped];
      if (!lifetime.active() || lifetime.version() !== version) return values;
      const count = (outcome: BulkResult["outcome"]) => values.filter((r) => r.outcome === outcome).length;
      toast(t("summary", { action: t(`actions.${action}`), started: count("started"), joined: count("joined"), skipped: count("skipped"), failed: count("failed") }), { action: { label: t("details"), onClick: () => { if (lifetime.active() && lifetime.version() === version) bulkResultStore.set({ results: values, open: true }); } } });
      return values;
    } finally { setRunning(false); }
  };
  return { run, running };
}
