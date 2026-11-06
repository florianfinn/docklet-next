import { useEffect } from "react";
import { identityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { useStore } from "@/shared/lib/store";
import { bulkResultStore } from "./bulk-result-store";
import { useBulkReason } from "@/shared/hooks/use-bulk-reason";
// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded bulk admission and captured result ownership. M19 web consumer integration.

import { useTranslations } from "use-intl";
import type { BulkResult } from "@/shared/hooks/bulk-run";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";

export function BulkResultDialog({ results, open, onOpenChange }: { results?: BulkResult[]; open: boolean; onOpenChange: (open: boolean) => void }) {
  const t = useTranslations("common.bulkRun"), reason = useBulkReason();
  return <Dialog open={open} onOpenChange={onOpenChange}><DialogContent><DialogHeader><DialogTitle>{t("details")}</DialogTitle><DialogDescription>{t("admissionHint")}</DialogDescription></DialogHeader><ul className="flex max-h-96 flex-col gap-3 overflow-y-auto">{results?.map((item) => <li key={item.id} className="flex flex-col gap-1"><span>{item.name} · {item.host}</span><span className="text-sm text-muted-foreground">{t(`outcomes.${item.outcome}`)}{item.code ? ` · ${reason(item.code, item.error)}` : ""}</span></li>)}</ul></DialogContent></Dialog>;
}

/** The shell owns Details even after the list that admitted the run has unmounted. */
export function BulkResultController() {
  const adapter = useAdapter(), state = useStore(bulkResultStore, (value) => value);
  useEffect(() => identityLifetime(adapter).listen(() => bulkResultStore.set({ open: false, results: [] })), [adapter]);
  return <BulkResultDialog results={state.results} open={state.open} onOpenChange={(open) => bulkResultStore.set((value) => ({ ...value, open }))} />;
}
