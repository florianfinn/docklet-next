import { Pause, Play } from "lucide-react";
import { useTranslations } from "use-intl";

import { simulationStore } from "@/data/demo/simulation";
import { useStore } from "@/shared/lib/store";
import { Button } from "@/shared/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@/shared/ui/tooltip";

export function LiveToggle() {
  const t = useTranslations("shell.live");
  const live = useStore(simulationStore, (s) => s.live);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className="h-8 gap-1.5 text-xs text-muted-foreground"
          aria-pressed={live}
          aria-label={live ? t("on") : t("off")}
          onClick={() => simulationStore.set((s) => ({ ...s, live: !s.live }))}
        >
          {live ? <Pause /> : <Play />}
          <span className="relative flex size-2" aria-hidden>
            {live && <span className="absolute inline-flex size-full animate-ping rounded-full bg-violet opacity-60" />}
            <span className={live ? "relative inline-flex size-2 rounded-full bg-violet" : "relative inline-flex size-2 rounded-full bg-muted-foreground"} />
          </span>
          <span className="hidden sm:inline">Live</span>
        </Button>
      </TooltipTrigger>
      <TooltipContent>{live ? t("on") : t("off")}</TooltipContent>
    </Tooltip>
  );
}
