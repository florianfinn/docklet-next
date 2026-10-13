import { useTranslations } from "use-intl";

import type { Host, Stack } from "@/domain/model";
import { UPDATE_TONE, updateVisual, type UpdateVisual } from "@/domain/updates";
import { HostChip } from "@/shared/components/host-chip";
import { useScheduleText } from "@/shared/components/schedule/use-schedule-text";
import { ToneDot } from "@/shared/components/tone";
import { UpdateIndicator } from "@/shared/components/updates/update-indicator";
import { cn } from "@/shared/lib/utils";
import { ScrollArea } from "@/shared/ui/scroll-area";

import { useStackCommands } from "@/shared/hooks/use-stack-commands";

const LANES: Exclude<UpdateVisual, "current" | "error" | "local" | "unchecked">[] = ["major", "minor", "patch", "rebuild"];

/** Updates grouped by kind (major/minor/patch/new image state) with hover details. */
export function UpdateRadarWidget({ stacks, hosts }: { stacks: Stack[]; hosts: Map<string, Host> }) {
  const t = useTranslations("dashboard.radar");
  const scheduleText = useScheduleText();
  const cmd = useStackCommands();
  const items = stacks.flatMap((s) =>
    s.services.map((svc) => ({ stack: s, svc, visual: updateVisual(svc.update) })).filter((x) => LANES.includes(x.visual as (typeof LANES)[number])),
  );
  const total = items.length;
  const scheduled = stacks.filter((s) => s.autoUpdate.mode === "scheduled" && s.services.some((x) => x.update.state === "version" || x.update.state === "rebuild"));

  if (!total) return <p className="text-sm text-muted-foreground">{t("none")}</p>;
  return (
    <div className="flex h-full flex-col gap-3">
      <div className="flex h-2.5 overflow-hidden rounded-full bg-muted" aria-hidden>
        {LANES.map((lane) => {
          const n = items.filter((x) => x.visual === lane).length;
          return n ? <span key={lane} data-tone={UPDATE_TONE[lane]} className="bg-(--tone) transition-[flex-grow] duration-(--dur-slow)" style={{ flexGrow: n }} /> : null;
        })}
      </div>
      <div className="flex flex-wrap gap-x-3 gap-y-1 text-xs text-muted-foreground">
        {LANES.map((lane) => (
          <span key={lane} className="inline-flex items-center gap-1.5">
            <ToneDot tone={UPDATE_TONE[lane]} />
            {t(lane)} <b className="text-foreground tabular">{items.filter((x) => x.visual === lane).length}</b>
          </span>
        ))}
      </div>
      <ScrollArea className="min-h-0 flex-1">
        <ul className="flex flex-col pr-2">
          {items.map(({ stack, svc, visual }) => (
            <li key={`${stack.id}-${svc.name}`} className={cn("flex items-center gap-2 border-b border-border/50 py-2 last:border-0")}>
              <ToneDot tone={UPDATE_TONE[visual]} />
              <div className="min-w-0 flex-1">
                <div className="truncate text-sm font-medium">
                  {svc.name} <span className="font-normal text-muted-foreground">· {stack.name}</span>
                </div>
                <HostChip host={hosts.get(stack.hostId)} />
              </div>
              <UpdateIndicator
                name={`${stack.name} / ${svc.name}`}
                update={svc.update}
                side="left"
                actions={{ onUpdate: () => void cmd.installUpdates(stack.id, [svc.name]), onRecheck: () => void cmd.checkUpdates(stack.id), busy: !!stack.busyActionId, updateLabel: undefined }}
              />
            </li>
          ))}
        </ul>
      </ScrollArea>
      <p className="text-[11px] text-muted-foreground">
        {scheduled.map((s) => t("auto", { stack: s.name, window: s.autoUpdate.mode === "scheduled" ? scheduleText.summary(s.autoUpdate.schedule) : "" })).join(" · ")}
        {scheduled.length > 0 && " · "}
        {t("manual")}
      </p>
    </div>
  );
}
