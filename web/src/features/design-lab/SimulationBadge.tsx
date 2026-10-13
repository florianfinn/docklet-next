import { FlaskConical } from "lucide-react";
import { useTranslations } from "use-intl";

import { simulationStore } from "@/data/demo/simulation";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

import { ToneDot } from "@/shared/components/tone";

/** Marks values that come from the metrics simulator, not a real host. */
export function SimulationBadge({ className }: { className?: string }) {
  const t = useTranslations("common.simulation");
  const live = useStore(simulationStore, (s) => s.live);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <span
          tabIndex={0}
          data-tone={live ? "violet" : "gray"}
          className={cn(
            "inline-flex h-6 items-center gap-1.5 rounded-lg border border-(--tone)/30 bg-(--tone)/10 px-2 text-[11px] font-medium text-(--tone) outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50",
            className,
          )}
        >
          <FlaskConical className="size-3" aria-hidden />
          {live ? t("badge") : t("paused")}
          {live && <ToneDot tone="violet" className="size-1.5 animate-breathe" />}
        </span>
      </TooltipTrigger>
      <TooltipContent>{t("hint")}</TooltipContent>
    </Tooltip>
  );
}
