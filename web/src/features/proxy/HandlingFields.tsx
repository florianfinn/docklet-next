// Source: web/src/features/design-lab/proxy/RouteFields.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Observed reference-kind chips, append order, lossless newline editor and frozen-chain rules.

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
  const observed = [...(read.middlewares ?? []), ...(read.routes ?? []).flatMap(r => r.references ?? [])];
  const chips = [...new Map(observed.filter(r => r.name !== "" && !r.name.includes("\n")).map(r => [r.name, r])).values()];
  return <>
    <FieldHelp label={t("existingMiddleware")} help={t("middlewareHint")}>
      <div role="group" aria-label={t("existingMiddleware")} className="flex flex-wrap gap-2">
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
