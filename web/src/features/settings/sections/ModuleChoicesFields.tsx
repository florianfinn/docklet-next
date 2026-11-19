// Source: docklet-next/issues/472
// Revision: 02c2a7139b3e0b1f3ec2790f403710de35ef051c
// Retrieved: 2026-11-15
// License: Apache-2.0
// Deviations: Original shared setup module switches; M29 #522 module cards.

import { Waypoints, type LucideIcon } from "lucide-react";
import { useTranslations } from "use-intl";
import { MODULES, type ModuleChoices, type ModuleID } from "@/domain/modules";
import { cn } from "@/shared/lib/utils";
import { Switch } from "@/shared/ui/switch";

const ICONS: Record<ModuleID, LucideIcon> = { proxy: Waypoints };

/** One card per module; the whole card is the switch's label and an enabled card gets the accent border. */
export function ModuleChoicesFields({ value, onChange }: { value: ModuleChoices; onChange: (value: ModuleChoices) => void }) {
  const t = useTranslations("settings.modules");
  return <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">{MODULES.map((m) => {
    const Icon = ICONS[m.id];
    const on = value[m.id];
    return <label key={m.id} htmlFor={`module-${m.id}`} data-slot="module-card" data-state={on ? "on" : "off"}
      className={cn("flex cursor-pointer flex-col gap-2 rounded-xl border bg-card/40 p-4 transition-colors hover:bg-accent/30", on && "border-primary")}>
      <span className="flex items-center gap-3">
        <Icon className="size-5 shrink-0 text-primary-text" aria-hidden />
        <span id={`module-${m.id}-name`} className="flex-1 text-sm font-medium">{t(`names.${m.id}`)}</span>
        <Switch id={`module-${m.id}`} aria-labelledby={`module-${m.id}-name`} aria-describedby={`module-${m.id}-description`} checked={on} onCheckedChange={(enabled) => onChange({ ...value, [m.id]: enabled })} />
      </span>
      <span id={`module-${m.id}-description`} className="text-sm text-muted-foreground">{t(`descriptions.${m.id}`)}</span>
    </label>;
  })}</div>;
}
