// Source: web/src/features/design-lab/proxy/controls.tsx
// Revision: 1d8ef389b0a40f3ba27fb09f8f59d8ac993180ff
// Retrieved: 2026-11-21
// License: Apache-2.0
// Deviations: Production-only section and detail presentation, anywhere wrapping and accessible field help; help button in the label row.

import type { ReactNode } from "react";
import { CircleHelp } from "lucide-react";
import { useTranslations } from "use-intl";
import { Tooltip, TooltipContent, TooltipProvider, TooltipTrigger } from "@/shared/ui/tooltip";

const tones = {
  domain: "border-cyan/25 bg-cyan/5", target: "border-primary/25 bg-primary/5",
  tls: "border-success/25 bg-success/5", behavior: "border-border bg-muted/20",
};
export function FormSection({ title, tone, children, disabled }: { title: string; tone: keyof typeof tones; children: ReactNode; disabled?: boolean }) {
  return <fieldset disabled={disabled} className={`grid min-w-0 gap-4 rounded-xl border p-4 ${tones[tone]}`}>
    <legend className="px-2 text-sm font-semibold">{title}</legend>{children}
  </fieldset>;
}
export function Detail({ label, children }: { label: string; children: ReactNode }) {
  return <div className="flex min-w-0 flex-wrap items-start justify-between gap-x-4 gap-y-1 border-b border-border/60 py-3 text-sm last:border-0">
    <dt className="text-muted-foreground">{label}</dt><dd className="min-w-0 whitespace-pre-wrap text-right [overflow-wrap:anywhere]">{children}</dd>
  </div>;
}
export function FieldHelp({ label, help, children }: { label: string; help: string; children: ReactNode }) {
  const t = useTranslations("proxyContract.ui");
  return <div className="relative min-w-0">
    {children}
    <TooltipProvider><Tooltip><TooltipTrigger asChild>
      <button type="button" className="absolute right-0 top-0 rounded-md p-1 text-muted-foreground focus-visible:ring-2 focus-visible:ring-ring" aria-label={t("fieldHelp", { name: label })}><CircleHelp className="size-4" /></button>
    </TooltipTrigger><TooltipContent className="max-w-72 [overflow-wrap:anywhere]">{help}</TooltipContent></Tooltip></TooltipProvider>
  </div>;
}
