// Source: web/src/features/design-lab/proxy/RouteFields.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Observed reference-kind chips, append order, lossless newline editor and frozen-chain rules. Usable references only; first kind wins; visible group label.

import { useTranslations } from "use-intl";
import { Label } from "@/shared/ui/label";
import { Textarea } from "@/shared/ui/textarea";
import { Button } from "@/shared/ui/button";
import { TextField } from "@/shared/components/form-field";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/shared/ui/tooltip";
import { FieldHelp } from "./presentation";
import type { ReadResult, Route } from "./types";

export function HandlingFields({ read, route, names, middlewares, frozen, update, priority, setPriority }: {
  read: ReadResult; route?: Route; names: string[]; middlewares: string; frozen: boolean;
  update: (s: string) => void; priority: string; setPriority: (s: string) => void;
}) {
  const t = useTranslations("proxyContract.ui");
  // Definitions first, then usable references of other routes; the first kind seen wins.
  const observed = [...(read.middlewares ?? []), ...(read.routes ?? []).flatMap(r => (r.references ?? []).filter(ref => !["opaque", "unresolved"].includes(ref.kind)))];
  const chips = [...observed.filter(r => r.name !== "" && !r.name.includes("\n")).reduce((m, r) => m.has(r.name) ? m : m.set(r.name, r), new Map<string, (typeof observed)[number]>()).values()];
  return <>
    <FieldHelp label={t("existingMiddleware")} help={t("middlewareHint")}>
      <p id="route-middleware-chips" className="mb-2 text-sm font-medium">{t("existingMiddleware")}</p>
      <div role="group" aria-labelledby="route-middleware-chips" className="flex flex-wrap gap-2">
        <TooltipProvider>{chips.map(ref => <Tooltip key={ref.name}><TooltipTrigger asChild>
          <Button type="button" size="sm" variant={names.includes(ref.name) ? "secondary" : "outline"} disabled={frozen} aria-pressed={names.includes(ref.name)}
            onClick={() => update((names.includes(ref.name) ? names.filter(n => n !== ref.name) : [...names, ref.name]).join("\n"))}>
            {ref.name}{names.includes(ref.name) && <span className="text-muted-foreground">{names.indexOf(ref.name) + 1}</span>}
          </Button>
        </TooltipTrigger><TooltipContent className="max-w-72">{t(`middlewareHelp.${ref.kind}`)}</TooltipContent></Tooltip>)}</TooltipProvider>
      </div>
    </FieldHelp>
    <FieldHelp label={t("middlewares")} help={t("middlewareHint")}>
      <div className="grid min-w-0 gap-2"><Label htmlFor="route-middlewares">{t("middlewares")}</Label>
        <Textarea id="route-middlewares" className="min-h-20 font-mono text-xs" value={middlewares} disabled={frozen} aria-describedby="route-middlewares-desc" spellCheck={false} onChange={e => update(e.target.value)} />
        <p id="route-middlewares-desc" className="text-xs text-muted-foreground">{t("middlewareHint")}</p>
        {frozen && <p className="text-xs text-muted-foreground">{t("frozenMiddlewares")}</p>}
      </div>
    </FieldHelp>
    <FieldHelp label={t("priority")} help={t("help.priority")}><TextField id="route-priority" label={t("priority")} type="number" value={priority} onChange={setPriority} /></FieldHelp>
    {route && (route.dependants?.length ?? 0) > 1 && <p className="text-xs text-muted-foreground">{t("sharedServiceHint")}</p>}
  </>;
}
