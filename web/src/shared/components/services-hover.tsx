import { useState } from "react";
import { ServiceHealthHint } from "@/shared/health/ServiceHealthHint";
import { Boxes } from "lucide-react";
import { useTranslations } from "use-intl";

import type { Host, Service } from "@/domain/model";
import { RUNTIME_TONE, serviceRuntime } from "@/domain/status";
import { versionLabel } from "@/domain/updates";
import { cn } from "@/shared/lib/utils";

import { InteractiveHoverCard } from "./interactive-hover-card";
import { ToneDot } from "./tone";

/** Service count with a hover list of service states and versions. */
export function ServicesHover({ services, host, name, stackId, className }: { stackId?: string; services: Service[]; host: Host | undefined; name: string; className?: string }) {
  const [open, setOpen] = useState(false);
  const t = useTranslations("common");
  const running = services.filter((s) => s.state === "running").length;
  const problem = services.some((s) => s.health === "unhealthy");
  return (
    <InteractiveHoverCard
      onOpenChange={setOpen}
      label={`${name}: ${t("services.label", { running, total: services.length })}`}
      contentClassName="w-72"
      content={
        <div className="p-3">
          <div className="mb-2 text-xs font-semibold text-muted-foreground">{t("services.title")}</div>
          <ul className="flex flex-col gap-1.5 text-xs">
            {services.map((s) => {
              const rt = serviceRuntime(s, host);
              return (
                <li key={s.name} className="flex flex-col gap-1">
                  <div className="flex items-center gap-2">
                    <ToneDot tone={RUNTIME_TONE[rt]} pulse={rt === "unhealthy"} />
                    <span className="min-w-0 flex-1 truncate font-medium">{s.name}</span>
                    <span className="font-mono text-[11px] text-muted-foreground">{s.buildOnly ? t("services.buildOnly") : versionLabel(s.image)}</span>
                  </div>
                  {stackId && s.health === "unhealthy" && <ServiceHealthHint containerId={s.containerId} stackId={stackId} serviceName={s.name} open={open} />}
                </li>
              );
            })}
          </ul>
        </div>
      }
    >
      <span
        data-tone={problem ? "err" : running === services.length ? "ok" : running === 0 ? "gray" : "warn"}
        className={cn("inline-flex items-center gap-1.5 rounded-md border border-dashed border-border px-1.5 py-0.5 text-xs tabular text-foreground hover:border-(--tone)/60", className)}
      >
        <Boxes className="size-3.5 text-(--tone)" aria-hidden />
        {t("services.count", { running, total: services.length })}
      </span>
    </InteractiveHoverCard>
  );
}
