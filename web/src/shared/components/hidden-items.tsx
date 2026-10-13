import { ChevronRight, Eye, EyeOff } from "lucide-react";
import type { ReactNode } from "react";
import { useTranslations } from "use-intl";

import { setHidden, type HiddenKind } from "@/shared/preferences/hidden-store";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";


export interface HiddenEntry {
  id: string;
  name: string;
  /** Short state, e.g. a status badge. */
  meta?: ReactNode;
  /** Opens the detail page or inspector; hidden entries stay reachable. */
  open: ReactNode;
}

/**
 * Collapsed "Ausgeblendet" category at the bottom of a host block or the
 * overview table. Closed by default; every entry can be shown again.
 */
export function HiddenSection({ kind, entries, title, className }: { kind: HiddenKind; entries: HiddenEntry[]; title?: string; className?: string }) {
  const t = useTranslations("common.hidden");
  if (!entries.length) return null;
  return (
    <Collapsible data-slot="hidden-section" className={className}>
      <div className="flex flex-wrap items-center gap-2 border-t border-dashed border-border/70 px-2 pt-2">
        <CollapsibleTrigger className="group/hidden flex min-w-0 items-center gap-1.5 rounded-md px-1.5 py-1 text-xs font-medium text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
          <ChevronRight className="size-3.5 transition-transform duration-(--dur-fast) group-data-[state=open]/hidden:rotate-90" aria-hidden />
          <EyeOff className="size-3.5" aria-hidden />
          {title ?? t("title")}
          <span className="rounded-full bg-muted px-1.5 text-[10.5px] tabular-nums">{entries.length}</span>
        </CollapsibleTrigger>
        <span className="flex-1" />
        <Button
          type="button"
          variant="ghost"
          size="sm"
          className="h-7 text-xs"
          onClick={() => {
            for (const e of entries) setHidden(kind, e.id, false);
          }}
        >
          <Eye data-icon="inline-start" />
          {t("restoreAll")}
        </Button>
      </div>
      <CollapsibleContent>
        <p className="px-3.5 pt-1 text-[11px] text-muted-foreground">{t("hint")}</p>
        <ul className="flex flex-col gap-1 px-1.5 py-1.5">
          {entries.map((e) => (
            <li key={e.id} data-hidden-entry={e.id} className="flex flex-wrap items-center gap-x-3 gap-y-1 rounded-lg border border-dashed border-border/70 bg-muted/15 px-2.5 py-1.5 text-sm">
              <span className="min-w-0 flex-1 font-medium [overflow-wrap:anywhere]">{e.open}</span>
              {e.meta}
              <Button type="button" size="sm" variant="outline" className="h-7 text-xs" onClick={() => setHidden(kind, e.id, false)} aria-label={t("restoreItem", { name: e.name })}>
                <Eye data-icon="inline-start" />
                {t("restore")}
              </Button>
            </li>
          ))}
        </ul>
      </CollapsibleContent>
    </Collapsible>
  );
}
