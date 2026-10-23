import { useCapability } from "@/data/use-capability";
import { useQueryClient } from "@tanstack/react-query";
import { Box, Layers, RefreshCw } from "lucide-react";
import { useMemo, useRef } from "react";
import { useTranslations } from "use-intl";

import { queryKeys } from "@/data/query-keys";
import { useContainers, useStacks } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import type { AutoUpdatePolicy } from "@/domain/model";
import type { HubSettings, UpdateInterval } from "@/domain/settings";
import { UPDATE_CHECK_ANCHOR, UPDATE_POLICIES_ANCHOR } from "@/domain/job-links";
import { DEFAULT_UPDATE_SCHEDULE } from "@/domain/schedule";
import { HostChip } from "@/shared/components/host-chip";
import { SchedulePopover } from "@/shared/components/schedule/schedule-popover";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { useHashFocus } from "@/shared/hooks/use-hash-focus";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";
import { HelpLabel, HelpTooltip } from "@/shared/help/help-tooltip";
import { Field, FieldContent, FieldDescription, FieldGroup } from "@/shared/ui/field";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { Skeleton } from "@/shared/ui/skeleton";
import { Switch } from "@/shared/ui/switch";
import { OverviewRowView, OverviewTable } from "@/shared/components/overview/overview-table";

import { SettingsForm } from "../SettingsForm";
import { useSaveSettings } from "../use-save-settings";
import { useSectionDraft } from "../use-section-draft";

interface Target {
  id: string;
  kind: "stack" | "container";
  name: string;
  hostId: string;
}

/** Update checks plus opt-in scheduled installs per stack and standalone container. */
export function UpdatesSection({ settings }: { settings: HubSettings }) {
  const t = useTranslations("settings");
  const tu = useTranslations("settings.updates");
  const adapter = useAdapter();
  const capability = useCapability("checkUpdates");
  const client = useQueryClient();
  const save = useSaveSettings();
  const stacks = useStacks();
  const containers = useContainers();
  const hosts = useHostScope();
  const timeZone = useHubTimeZone();

  const targets = useMemo<Target[]>(
    () => [
      ...(stacks.data ?? []).map((s) => ({ id: s.id, kind: "stack" as const, name: s.name, hostId: s.hostId })),
      ...(containers.data ?? []).filter((c) => c.origin === "standalone").map((c) => ({ id: c.id, kind: "container" as const, name: c.name, hostId: c.hostId })),
    ],
    [stacks.data, containers.data],
  );
  const savedPolicies = useMemo(() => {
    const out: Record<string, AutoUpdatePolicy> = {};
    for (const s of stacks.data ?? []) out[s.id] = s.autoUpdate;
    for (const c of containers.data ?? []) if (c.origin === "standalone") out[c.id] = c.autoUpdate ?? { mode: "check" };
    return out;
  }, [stacks.data, containers.data]);
  const loaded = !!stacks.data && !!containers.data;
  const section = useSectionDraft(loaded ? { updates: settings.updates, policies: savedPolicies } : undefined);
  const draft = section.draft;

  // Scheduled-job links from the dashboard focus the check interval or the policy table.
  const checkRef = useRef<HTMLDivElement>(null);
  const policiesRef = useRef<HTMLElement>(null);
  useHashFocus(UPDATE_CHECK_ANCHOR, checkRef, !!draft);
  useHashFocus(UPDATE_POLICIES_ANCHOR, policiesRef, !!draft);

  const setPolicy = (id: string, policy: AutoUpdatePolicy) => draft && section.patch({ policies: { ...draft.policies, [id]: policy } });

  return (
    <SettingsForm
      title={t("sections.updates")}
      dirty={section.dirty}
      onDiscard={section.reset}
      onSave={async () => {
        if (!draft) return;
        await save("updates", draft.updates);
        for (const tg of targets) {
          const next = draft.policies[tg.id];
          if (!next || JSON.stringify(next) === JSON.stringify(savedPolicies[tg.id])) continue;
          if (tg.kind === "stack") await adapter.setAutoUpdate(tg.id, next);
          else await adapter.setContainerAutoUpdate(tg.id, next);
        }
        await client.invalidateQueries({ queryKey: queryKeys.stacks });
        await client.invalidateQueries({ queryKey: queryKeys.containers });
        section.commit(draft);
      }}
    >
      {!draft ? (
        <Skeleton className="h-64 rounded-xl" />
      ) : (
        <FieldGroup>
          <div ref={checkRef} id={UPDATE_CHECK_ANCHOR} tabIndex={-1} className="flex scroll-mt-24 flex-wrap items-end gap-4 rounded-lg outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
            <Field className="w-full sm:w-60">
              <HelpLabel htmlFor="upd-interval" topic="settingsUpdates.interval">
                {tu("interval")}
              </HelpLabel>
              <Select value={draft.updates.interval} onValueChange={(v) => section.patch({ updates: { ...draft.updates, interval: v as UpdateInterval } })}>
                <SelectTrigger id="upd-interval" className="w-full">
                  <SelectValue />
                </SelectTrigger>
                <SelectContent>
                  <SelectGroup>
                    {(["6h", "12h", "24h"] as const).map((i) => (
                      <SelectItem key={i} value={i}>
                        {tu(`intervals.${i}`)}
                      </SelectItem>
                    ))}
                  </SelectGroup>
                </SelectContent>
              </Select>
            </Field>
            <Button type="button" variant="outline" className="group" disabled={!capability.available} title={capability.reason} onClick={() => void adapter.checkUpdates({})}>
              <RefreshCw data-icon="inline-start" className="spin-on-hover" />
              {tu("checkNow")}
            </Button>
          </div>
          <Field orientation="horizontal">
            <Switch id="upd-notify" checked={draft.updates.notify} onCheckedChange={(notify) => section.patch({ updates: { ...draft.updates, notify } })} />
            <FieldContent>
              <HelpLabel htmlFor="upd-notify" topic="settingsUpdates.notify">
                {tu("notify")}
              </HelpLabel>
              <FieldDescription>{tu("notifyHint")}</FieldDescription>
            </FieldContent>
          </Field>
          <Alert>
            <RefreshCw />
            <AlertDescription className="text-xs">
              {tu("defaultMode")} {tu("noRollback")}
            </AlertDescription>
          </Alert>
          <section ref={policiesRef} id={UPDATE_POLICIES_ANCHOR} tabIndex={-1} aria-labelledby="upd-table" className="flex scroll-mt-24 flex-col gap-2 rounded-lg outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50">
            <div className="flex items-center gap-1">
              <h3 id="upd-table" className="text-sm font-semibold">
                {tu("table")}
              </h3>
              <HelpTooltip topic="settingsUpdates.autoInstall" />
            </div>
            <OverviewTable
              fitKey={`updates|${targets.length}`}
              columns={() => [
                { id: "name", label: tu("target"), className: "min-w-40" },
                { id: "mode", label: tu("mode") },
                { id: "window", label: tu("window"), wide: true },
              ]}
              selecting={false}
              actionsLabel=""
              dense
              listLabel={tu("table")}
            >
              {(view, cols) =>
                targets.map((tg) => {
                  const p = draft.policies[tg.id] ?? { mode: "check" };
                  return (
                    <OverviewRowView
                      key={tg.id}
                      view={view}
                      columns={cols}
                      pad="py-2"
                      hostColor={hosts.byId.get(tg.hostId)?.color}
                      className="cursor-default"
                      actions={null}
                      cells={{
                        name: (
                          <span className="flex flex-col gap-0.5">
                            <span className="flex items-center gap-2 font-medium">
                              {tg.kind === "stack" ? <Layers className="size-3.5 text-muted-foreground" aria-hidden /> : <Box className="size-3.5 text-muted-foreground" aria-hidden />}
                              {tg.name}
                            </span>
                            <HostChip host={hosts.byId.get(tg.hostId)} />
                          </span>
                        ),
                        mode: (
                          <SegmentedToggle
                            value={p.mode}
                            onChange={(mode) => setPolicy(tg.id, mode === "check" ? { mode } : { mode, schedule: DEFAULT_UPDATE_SCHEDULE })}
                            label={`${tu("mode")}: ${tg.name}`}
                            options={[
                              { value: "check", label: tu("check") },
                              { value: "scheduled", label: tu("scheduled") },
                            ]}
                          />
                        ),
                        window:
                          p.mode === "scheduled" ? (
                            <SchedulePopover id={`upd-schedule-${tg.id}`} value={p.schedule} label={tg.name} timeZone={timeZone} onApply={(schedule) => setPolicy(tg.id, { mode: "scheduled", schedule })} />
                          ) : (
                            <span className="text-xs text-muted-foreground">–</span>
                          ),
                      }}
                    />
                  );
                })
              }
            </OverviewTable>
            <p className="text-xs text-muted-foreground">{tu("excluded")}</p>
          </section>
        </FieldGroup>
      )}
    </SettingsForm>
  );
}
