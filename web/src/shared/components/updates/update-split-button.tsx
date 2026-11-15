// Source: docklet-next/issues/440
// Revision: 6e3afa6a78f4f60ffd74858d9a1eaefbb1b73bda
// Retrieved: 2026-11-12
// License: Apache-2.0
// Deviations: Original observed update UI and regression coverage.

import { ChevronDown } from "lucide-react";
import { useId } from "react";
import { useTranslations } from "use-intl";
import { Button } from "@/shared/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuGroup, DropdownMenuItem, DropdownMenuTrigger } from "@/shared/ui/dropdown-menu";

export function UpdateSplitButton({ label, disabled, title, onUpdate, onChecked }: { label: string; disabled?: boolean; title?: string; onUpdate: () => void; onChecked: () => void }) {
  const t = useTranslations("common.stackUpdates"), help = useId();
  return <div className="inline-flex min-w-0" role="group" aria-label={label}>
    <Button type="button" className="rounded-r-none" disabled={disabled} title={title} onClick={onUpdate}>{label}</Button>
    <DropdownMenu><DropdownMenuTrigger asChild><Button type="button" className="rounded-l-none border-l border-primary-foreground/30 px-2" disabled={disabled} title={title} aria-label={t("updateOptions")}><ChevronDown aria-hidden className="size-4" /></Button></DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="max-w-80"><DropdownMenuGroup><DropdownMenuItem aria-describedby={help} onSelect={onChecked}>{t("updateWithCheck")}</DropdownMenuItem></DropdownMenuGroup><p id={help} className="px-2 py-2 text-xs text-muted-foreground">{t("updateWithCheckHelp")}</p></DropdownMenuContent>
    </DropdownMenu>
  </div>;
}
