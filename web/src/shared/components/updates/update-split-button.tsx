// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { ChevronDown, ShieldCheck } from "lucide-react";
import { useId } from "react";
import { useTranslations } from "use-intl";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";

export function UpdateSplitButton({ label, disabled, title, onUpdate, onChecked }: { label: string; disabled?: boolean; title?: string; onUpdate: () => void; onChecked: () => void }) {
  const t = useTranslations("common.stackUpdates"), name = useId(), hint = useId();
  return <div className="inline-flex min-w-0" role="group" aria-label={label}>
    <Button type="button" className="rounded-r-none" disabled={disabled} title={title} onClick={onUpdate}>{label}</Button>
    <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" className="rounded-l-none border-l border-primary-foreground/30 px-2" disabled={disabled} title={title} aria-label={t("updateOptions")}><ChevronDown aria-hidden className="size-4" /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-w-80"><DropdownMenuGroup>
        {/* The menu holds only items; the full explanation is in the confirmation dialog. */}
        <DropdownMenuItem aria-labelledby={name} aria-describedby={hint} onSelect={onChecked}><ShieldCheck aria-hidden /><span className="flex min-w-0 flex-col"><span id={name}>{t("updateWithCheck")}</span><span id={hint} className="truncate text-xs text-muted-foreground">{t("updateWithCheckHint")}</span></span></DropdownMenuItem>
      </DropdownMenuGroup></DropdownMenuContent>
    </DropdownMenu>
  </div>;
}
