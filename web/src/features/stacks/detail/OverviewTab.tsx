import { useAdapter } from "@/data/use-adapter";
import { LiveStackUpdates } from "@/shared/components/updates/live-stack-updates";
import { CapabilityGate } from "@/shared/components/capability-gate";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { CalendarClock, HeartPulse, History, Info } from "lucide-react";
import { useRef } from "react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { useStacks } from "@/data/queries";
import type { Host, StackDetail } from "@/domain/model";
import { IconTile } from "@/shared/components/tone";
import { StackUpdatePanel } from "@/shared/components/updates/update-panel";
import { useStackUpdateTimes } from "@/shared/components/updates/update-times";
import { useAdapterText } from "@/data/use-adapter-text";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { AUTO_UPDATE_ANCHOR } from "@/domain/job-links";
import { DEFAULT_UPDATE_SCHEDULE } from "@/domain/schedule";
import { SchedulePopover } from "@/shared/components/schedule/schedule-popover";
import { useHashFocus } from "@/shared/hooks/use-hash-focus";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";

import { StorageCard } from "@/features/volumes/StorageCard";

import { LabelEditor } from "../editor/fields";
import { allLabels } from "../overview/stack-filters";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { ServicesCard } from "./ServicesCard";

export function OverviewTab({ stack, host }: { stack: StackDetail; host: Host | undefined }) {
  const t = useTranslations("stacks.overview");
  const adapter = useAdapter();
  const th = useTranslations("common.host");
  const dt = useAdapterText();
  const timeZone = useHubTimeZone();
  const navigate = useNavigate();
  const cmd = useStackCommands();
  const stacks = useStacks();
  const unhealthy = stack.services.filter((s) => s.health === "unhealthy").map((s) => s.name);
  const busy = !!stack.busyActionId;
  const updateTimes = useStackUpdateTimes(stack);
  const autoRef = useRef<HTMLDivElement>(null);
  useHashFocus(AUTO_UPDATE_ANCHOR, autoRef);

  return (
    <div className="flex flex-col gap-4">
      <ServicesCard stack={stack} host={host} />
      <div className="grid gap-4 lg:grid-cols-3">
        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2.5">
              <IconTile tone={unhealthy.length ? "err" : "ok"} size="sm">
                <HeartPulse />
              </IconTile>
              {t("health")}
            </CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 px-4 text-sm">
            {unhealthy.length ? (
              <>
                <p className="text-muted-foreground">{t("healthBad", { names: unhealthy.join(", ") })}</p>
                <div className="flex flex-wrap gap-2">
                  <Button size="sm" variant="outline" onClick={() => navigate(`/stacks/${stack.id}?tab=logs&service=${unhealthy[0]}`)}>
                    {t("openLogs")}
                  </Button>
                  {stack.restorePoints.length > 0 && (
                    <Button size="sm" variant="outline" onClick={() => navigate(`/stacks/${stack.id}?tab=deployments`)}>
                      <History data-icon="inline-start" /> {t("openRestore")}
                    </Button>
                  )}
                </div>
              </>
            ) : (
              <p className="text-muted-foreground">{t("healthOk")}</p>
            )}
          </CardContent>
        </Card>

        <Card className="gap-0 overflow-hidden py-0">
          <StackUpdatePanel services={stack.services} times={updateTimes} actions={{ onUpdate: () => void cmd.installUpdates(stack.id), onRecheck: () => void cmd.checkUpdates(stack.id), busy }} />
          <div ref={autoRef} id={AUTO_UPDATE_ANCHOR} tabIndex={-1} className="flex scroll-mt-24 flex-col gap-2 border-t p-4 outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:ring-inset">
            <div className="flex items-center gap-2 text-sm font-medium">
              <CalendarClock className="size-4 text-muted-foreground" aria-hidden />
              {t("autoTitle")}
            </div>
            {adapter.kind === "http" ? <LiveStackUpdates key={stack.id} stackId={stack.id} /> : <CapabilityGate operation="setAutoUpdate">
            <SegmentedToggle
              value={stack.autoUpdate.mode}
              onChange={(v) => void cmd.setAutoUpdate(stack.id, v === "check" ? { mode: "check" } : { mode: "scheduled", schedule: DEFAULT_UPDATE_SCHEDULE })}
              label={t("autoTitle")}
              options={[
                { value: "check", label: t("autoCheck") },
                { value: "scheduled", label: t("autoScheduled") },
              ]}
              className="self-start"
            />
            {stack.autoUpdate.mode === "scheduled" && (
              <div className="flex flex-col gap-1">
                <span className="text-xs text-muted-foreground">{t("window")}</span>
                <SchedulePopover id="auto-window" value={stack.autoUpdate.schedule} label={stack.name} timeZone={timeZone} onApply={(schedule) => void cmd.setAutoUpdate(stack.id, { mode: "scheduled", schedule })} className="self-start" />
              </div>
            )}
            <p className="text-xs text-muted-foreground">{t("autoHint")}</p>
            </CapabilityGate>}
          </div>
        </Card>

        <Card className="gap-3 py-4">
          <CardHeader className="px-4">
            <CardTitle className="flex items-center gap-2.5">
              <IconTile tone="accent" size="sm">
                <Info />
              </IconTile>
              {t("details")}
            </CardTitle>
            <CardDescription>{dt(stack.description)}</CardDescription>
          </CardHeader>
          <CardContent className="flex flex-col gap-3 px-4">
            <dl className="grid grid-cols-[auto_1fr] gap-x-4 gap-y-2 text-sm">
              <dt className="text-muted-foreground">{t("host")}</dt>
              <dd className="flex items-center gap-1.5">
                <i className="size-2.5 rounded-[3px]" style={{ background: host?.color }} aria-hidden />
                {host?.name} · {host?.connection === "edge" ? th("edge") : th("direct")}
              </dd>
              <dt className="text-muted-foreground">{t("projectDir")}</dt>
              <dd className="font-mono text-xs break-all">
                {stack.path ?? `${host?.projectDir}/${stack.name}`}
              </dd>
              {stack.url && (
                <>
                  <dt className="text-muted-foreground">{t("url")}</dt>
                  <dd className="truncate">{stack.url}</dd>
                </>
              )}
            </dl>
            <div>
              <div className="mb-1.5 text-sm text-muted-foreground">{t("labels")}</div>
              <LabelEditor
                labels={stack.labels}
                onChange={(labels) => void cmd.setLabels(stack.id, labels)}
                suggestions={allLabels(stacks.data ?? [])}
                placeholder="produktiv"
                addLabel={t("labelsAdd")}
                removeLabel={(label) => t("labelRemove", { label })}
                emptyText={t("labelsNone")}
              />
            </div>
          </CardContent>
        </Card>
      </div>
      <StorageCard stackId={stack.id} />
    </div>
  );
}
