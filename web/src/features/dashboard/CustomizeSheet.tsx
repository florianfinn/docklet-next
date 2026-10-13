import { Check, Plus, RotateCcw, Settings2 } from "lucide-react";
import type { ReactNode } from "react";
import { flushSync } from "react-dom";
import { useTranslations } from "use-intl";

import type { Tone } from "@/domain/model";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Sheet, SheetContent, SheetDescription, SheetFooter, SheetHeader, SheetTitle } from "@/shared/ui/sheet";

import { CORE_WIDGETS, DEFAULT_LAYOUT, layoutStore, SERVICE_KINDS, toggleWidget, useLayout, withTransition, type WidgetId } from "./dashboard-layout";
import { AppIcon } from "./services/app-icons";

interface CatalogProps {
  open: boolean;
  onOpenChange: (o: boolean) => void;
  animate: boolean;
  meta: Record<string, { icon: ReactNode; tone: Tone }>;
  onConfigure?: (kind: (typeof SERVICE_KINDS)[number]) => void;
}

function CatalogItem({ shown, onToggle, icon, title, desc, extra }: { shown: boolean; onToggle: () => void; icon: ReactNode; title: string; desc: string; extra?: ReactNode }) {
  const t = useTranslations("dashboard.customizeSheet");
  return (
    <li className={cn("flex items-start gap-3 rounded-xl border p-3 transition-colors", shown && "border-primary/40 bg-primary/6")}>
      {icon}
      <div className="min-w-0 flex-1">
        <div className="text-sm font-medium">{title}</div>
        <div className="text-xs text-muted-foreground">{desc}</div>
        {extra}
      </div>
      <Button size="sm" variant={shown ? "secondary" : "outline"} aria-pressed={shown} onClick={onToggle} aria-label={shown ? t("hide", { name: title }) : t("show", { name: title })}>
        {shown ? <Check data-icon="inline-start" /> : <Plus data-icon="inline-start" />}
        {shown ? t("added") : t("add")}
      </Button>
    </li>
  );
}

/**
 * Widget catalog of the dashboard edit mode: add or remove operations and
 * service widgets. Order is changed on the dashboard itself (drag, arrows).
 */
export function CustomizeSheet({ open, onOpenChange, animate, meta, onConfigure }: CatalogProps) {
  const t = useTranslations("dashboard");
  const ts = useTranslations("dashboard.services");
  const layout = useLayout();
  const apply = (fn: () => void) => withTransition(() => flushSync(fn), animate);
  const toggle = (id: WidgetId) => apply(() => layoutStore.set((l) => toggleWidget(l, id)));

  return (
    <Sheet open={open} onOpenChange={onOpenChange}>
      <SheetContent className="w-full sm:max-w-md">
        <SheetHeader>
          <SheetTitle>{t("customizeSheet.title")}</SheetTitle>
          <SheetDescription>{t("customizeSheet.description")}</SheetDescription>
        </SheetHeader>
        <div className="flex min-h-0 flex-col gap-5 overflow-auto px-4">
          {import.meta.env.DEV && <section aria-labelledby="cat-services">
            <h3 id="cat-services" className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              {t("customizeSheet.services")}
            </h3>
            <ul className="flex flex-col gap-2">
              {SERVICE_KINDS.map((k) => {
                const cfg = layout.services[k];
                return (
                  <CatalogItem
                    key={k}
                    shown={!layout.hidden.includes(k)}
                    onToggle={() => toggle(k)}
                    icon={<AppIcon kind={k} className="grid size-9 shrink-0 overflow-hidden rounded-xl" />}
                    title={ts(`${k}.name`)}
                    desc={ts(`${k}.desc`)}
                    extra={
                      <div className="mt-1.5 flex flex-wrap items-center gap-2">
                        <ToneBadge tone={cfg.status === "connected" ? "ok" : cfg.status === "error" ? "err" : "gray"} className="h-5 px-1.5 text-[10.5px]">
                          {ts(`status.${cfg.status}`)}
                        </ToneBadge>
                        {onConfigure && <Button size="sm" variant="ghost" className="h-6 px-2 text-xs" onClick={() => onConfigure(k)}>
                          <Settings2 data-icon="inline-start" />
                          {ts("configure")}
                        </Button>}
                      </div>
                    }
                  />
                );
              })}
            </ul>
            {import.meta.env.DEV && <p className="mt-2 text-[11px] text-muted-foreground">{ts("demoNote")}</p>}
          </section>}
          <section aria-labelledby="cat-core">
            <h3 id="cat-core" className="mb-2 text-xs font-semibold tracking-wide text-muted-foreground uppercase">
              {t("customizeSheet.core")}
            </h3>
            <ul className="flex flex-col gap-2">
              {CORE_WIDGETS.map((id) => (
                <CatalogItem
                  key={id}
                  shown={!layout.hidden.includes(id)}
                  onToggle={() => toggle(id)}
                  icon={
                    <IconTile tone={meta[id]?.tone ?? "gray"} size="md">
                      {meta[id]?.icon}
                    </IconTile>
                  }
                  title={t(`widget.${id}.title`)}
                  desc={t(`widget.${id}.desc`)}
                />
              ))}
            </ul>
          </section>
          <p className="text-xs text-muted-foreground">{t("customizeSheet.keyboard")}</p>
        </div>
        <SheetFooter>
          <Button variant="outline" onClick={() => apply(() => layoutStore.set((l) => ({ ...DEFAULT_LAYOUT, services: l.services })))}>
            <RotateCcw data-icon="inline-start" /> {t("customizeSheet.reset")}
          </Button>
        </SheetFooter>
      </SheetContent>
    </Sheet>
  );
}
