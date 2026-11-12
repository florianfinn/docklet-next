// Source: docklet-next/issues/349
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original bounded bulk admission and captured result ownership. M19 web consumer integration.

import { useEffect, useState } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { readBulkUpdates, type BulkItem, type BulkResult, type BulkUpdateItem } from "@/shared/hooks/bulk-run";
import { useBulkReason } from "@/shared/hooks/use-bulk-reason";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Button } from "@/shared/ui/button";

export function BulkUpdateDialog({ items, onClose, install }: { items: readonly BulkItem[]; onClose: () => void; install: (items: BulkItem[], revisions: Map<string, string>, skipped: BulkResult[]) => Promise<unknown> }) {
  const adapter = useAdapter(), t = useTranslations("common.bulkRun"), reason = useBulkReason();
  const [rows, setRows] = useState<BulkUpdateItem[]>(), [busy, setBusy] = useState(false);
  useEffect(() => {
    let current = true;
    const off = identityLifetime(adapter).listen(() => { current = false; onClose(); });
    void readBulkUpdates(adapter, items).then((value) => { if (current) setRows(value); });
    return () => { current = false; off(); };
  }, [adapter, items, onClose]);
  const eligible = rows?.filter((row) => row.revision) ?? [];
  return <Dialog open onOpenChange={(open) => !open && onClose()}><DialogContent><DialogHeader><DialogTitle>{t("actions.update")}</DialogTitle><DialogDescription>{t("updateHint")}</DialogDescription></DialogHeader>{!rows ? <p role="status">{t("loading")}</p> : <ul className="flex max-h-96 flex-col gap-3 overflow-y-auto">{rows.map((row) => <li key={row.item.id}>{row.item.name} · {row.item.host}<p className="text-sm text-muted-foreground">{row.revision ? t("candidates", { count: row.count }) : reason(row.code, row.error)}</p></li>)}</ul>}<DialogFooter><Button disabled={busy || !eligible.length} onClick={async () => { setBusy(true); try { await install(eligible.map((row) => row.item), new Map(eligible.map((row) => [row.item.id, row.revision!])), rows?.filter((row) => !row.revision).map((row) => ({ ...row.item, outcome: "skipped", code: row.code, error: row.error })) ?? []); onClose(); } finally { setBusy(false); } }}>{t("install")}</Button></DialogFooter></DialogContent></Dialog>;
}
