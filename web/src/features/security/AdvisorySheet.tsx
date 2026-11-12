// Source: docklet-next/issues/437
// Revision: 2c49b8db789d39b644e27a85a09e277707b4fe12
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original URL-owned detail sheet with explicit invoking-row focus restoration. M21 #437: advisory detail.

import { useEffect, useRef, type RefObject } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import type { Host } from "@/domain/model";
import { advisoryLabel } from "./advisory-selection";
import { Button } from "@/shared/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { AdvisoryContent } from "./AdvisoryContent";
import { useAdvisoryDetail } from "./use-advisory-detail";


export function AdvisorySheet({ finding, selected, hostIds, hosts, origin, onClose }: { finding: string | null; selected: string | null; hostIds: readonly string[]; hosts: Map<string, Host>; origin: RefObject<HTMLElement | null>; onClose: () => void }) {
  const t = useTranslations("security"), detail = useAdvisoryDetail(finding, hostIds);
  const lastFinding = useRef(finding);
  useEffect(() => { if (finding) lastFinding.current = finding; }, [finding]);
  const label = finding ? advisoryLabel(finding) : "";
  return <Sheet open={!!finding} onOpenChange={(open) => { if (!open) onClose(); }}>
    <SheetContent className="w-full overflow-y-auto sm:max-w-2xl" onCloseAutoFocus={(event) => {
      event.preventDefault();
      const target = origin.current?.isConnected ? origin.current : document.querySelector<HTMLElement>(`[data-advisory-key="${lastFinding.current && /^[A-Za-z0-9_-]+$/.test(lastFinding.current) ? lastFinding.current : ""}"]`) ?? document.querySelector<HTMLElement>("[data-slot=security-table]");
      target?.focus({ preventScroll: true });
    }}>
      <SheetHeader>
        <SheetTitle>{label || t("detail.title")}</SheetTitle>
        <SheetDescription>{hostIds.length ? t("detail.scopeSelected", { hosts: hostIds.map((id) => hosts.get(id)?.name ?? id).join(", ") }) : t("detail.scopeAll")}</SheetDescription>
        {finding && label && <Button asChild variant="outline" size="sm"><Link to={`/security/vulnerabilities/${finding}${selected ? `?occ=${selected}` : ""}`}>{t("detail.openPage")}</Link></Button>}
      </SheetHeader>
      <div className="px-4 pb-6"><AdvisoryContent {...detail} hosts={hosts} selected={selected} onRetry={detail.reload} /></div>
    </SheetContent>
  </Sheet>;
}
