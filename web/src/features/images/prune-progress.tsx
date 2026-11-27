// Source: docklet-next/docs/design/image-prune-progress.md
// Revision: b637b8c19497e52b7a62987a497ca4ee05b1b5af
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original per-image cleanup progress and compact counts for issue #661.

import type { ReactNode } from "react";
import { useTranslations } from "use-intl";
import { ChevronRight } from "lucide-react";
import type { ActionRun, BulkCheckCounts, Host } from "@/domain/model";
import { usePruneCountText } from "./use-prune-runs";
import { Progress } from "@/shared/ui/progress";
import { Spinner } from "@/shared/ui/spinner";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";

/** One row per host while the cleanup runs: a bar once the host reports its plan, a spinner before. */
export function PruneProgress({ hosts, runs }: { hosts: Host[]; runs: ReadonlyMap<string, ActionRun> }) {
  const t = useTranslations("images.pruneDialog"), counts = usePruneCountText();
  const known = hosts.map((h) => runs.get(h.id)?.bulk).filter((b): b is BulkCheckCounts => !!b);
  const planned = known.reduce((n, b) => n + b.planned, 0), done = known.reduce((n, b) => n + b.completed, 0);
  return <div className="flex flex-col gap-3 text-sm" role="status" aria-live="polite" data-slot="prune-progress">
    <p className="font-medium">{known.length === hosts.length && planned > 0 ? t("progressTotal", { done, planned }) : t("progressStarting")}</p>
    {hosts.map((host) => {
      const bulk = runs.get(host.id)?.bulk;
      return <div key={host.id} className="flex flex-col gap-1" data-host={host.id}>
        <div className="flex items-center justify-between gap-2"><span className="truncate">{host.name}</span><span className="text-muted-foreground tabular-nums">{bulk ? counts(bulk, true) : <Spinner className="size-3.5" />}</span></div>
        {bulk && <Progress value={bulk.planned ? (bulk.completed * 100) / bulk.planned : 0} aria-label={host.name} />}
      </div>;
    })}
  </div>;
}

/** A collapsed name list; only the count is visible until the user opens it. */
export function NameDetails({ count, children }: { count: number; children: ReactNode }) {
  const t = useTranslations("images.pruneDialog");
  if (count === 0) return null;
  return <Collapsible>
    <CollapsibleTrigger className="group flex items-center gap-1 text-xs text-muted-foreground hover:text-foreground"><ChevronRight className="size-3.5 transition-transform group-data-[state=open]:rotate-90" aria-hidden />{t("details", { count })}</CollapsibleTrigger>
    <CollapsibleContent><ul className="mt-1 max-h-48 overflow-auto text-xs">{children}</ul></CollapsibleContent>
  </Collapsible>;
}
