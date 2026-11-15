// Source: docklet-next/issues/341
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original shared v14 policy controls for stack and container targets. M24 S6: observed recovery and update UI follow-ups.

import type { ReactNode } from "react";
import { ToggleGroup, ToggleGroupItem } from "@/shared/ui/toggle-group";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { StackUpdatePolicy, UpdateWindow, PolicyMode } from "@/domain/stack-updates";
import type { Schedule } from "@/domain/schedule";
import { SchedulePopover } from "@/shared/components/schedule/schedule-popover";
import { Button } from "@/shared/ui/button";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { useUpdateAction } from "./use-update-action";

const DAYS = ["sun", "mon", "tue", "wed", "thu", "fri", "sat"] as const;
function windowSchedule(window: UpdateWindow): Schedule {
  return window.kind === "daily" ? { mode: "daily", time: window.time } : { mode: "weekly", time: window.time, day: DAYS[window.day ?? 0]! };
}
export function PolicyControl({ policy, timeZone, saved, render }: { policy: StackUpdatePolicy; timeZone: string; saved: () => void; render?: (mode: ReactNode, window: ReactNode) => ReactNode }) {
  const adapter = useAdapter(), t = useTranslations("settings.updates"), common = useTranslations("common.stackUpdates");
  const id = `${policy.environmentId}~${policy.targetId}`, ref = { kind: policy.targetKind, id };
  const check = useCapability("setAutoUpdate", ref), scheduled = useCapability("setAutoUpdateApply", ref);
  const action = useUpdateAction(`${id}:${policy.registrationId}:${policy.targetIdentity}`);
  const save = (mode: PolicyMode, window = policy.window ?? { kind: "weekly" as const, day: 0, time: "04:00" }) => void action.run(() => policy.targetKind === "stack" ? adapter.saveAutoUpdatePolicy!(id, mode, policy.revision, window) : adapter.saveContainerUpdatePolicy!(id, mode, policy.revision, window), saved);
  const mode = <div className="flex min-w-0 flex-wrap items-start gap-2">
    {render ? <ToggleGroup type="single" variant="outline" value={policy.mode} disabled={action.busy || !check.available} aria-label={`${t("mode")}: ${policy.targetName}`} onValueChange={(mode) => { if (mode) save(mode as PolicyMode); }}>
      <ToggleGroupItem value="check">{t("check")}</ToggleGroupItem><ToggleGroupItem value="scheduled" disabled={!scheduled.available}>{t("scheduled")}</ToggleGroupItem>
    </ToggleGroup> : <>
    <Select value={policy.mode} disabled={action.busy || !check.available} onValueChange={(mode) => save(mode as PolicyMode)}>
      <SelectTrigger className="w-auto min-w-36 max-w-full" aria-label={`${t("mode")}: ${policy.targetName}`}><SelectValue /></SelectTrigger>
      <SelectContent><SelectGroup><SelectItem value="check">{t("check")}</SelectItem><SelectItem value="scheduled" disabled={!scheduled.available}>{t("scheduled")}</SelectItem></SelectGroup></SelectContent>
    </Select>
    </>}
    {policy.targetMissing && <div><p className="text-xs">{t("missing")}</p><Button type="button" variant="outline" size="sm" disabled={!check.available || action.busy} onClick={() => save("check")}>{t("remove")}</Button></div>}
    {action.error && <p role="alert" className="basis-full text-xs">{action.error}</p>}
    {!check.available && <p className="basis-full text-xs text-muted-foreground">{check.reason}</p>}
    <span className="sr-only">{common("policyHelp")}</span>
  </div>;
  const window = policy.mode === "check" ? <span>–</span> : <>{policy.mode === "scheduled" && policy.window && <fieldset disabled={!scheduled.available || action.busy} className="min-w-0">
      <SchedulePopover id={`window-${policy.targetKind}-${id}`} value={windowSchedule(policy.window)} modes={["daily", "weekly"]} label={policy.targetName} timeZone={timeZone} onApply={(s) => { if (s.mode !== "once") save("scheduled", { kind: s.mode, time: s.time, ...(s.mode === "weekly" ? { day: DAYS.indexOf(s.day) } : {}) }); }} />
    </fieldset>}</>;
  return render ? render(mode, window) : <div className="flex flex-wrap items-start gap-2">{mode}{policy.mode === "scheduled" && window}</div>;
}
