import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { CheckCircle2, ChevronDown, History, ListChecks, Loader2, Maximize2, XCircle } from "lucide-react";
import { useEffect, useMemo, useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";

import { useActionRuns } from "@/data/live";
import { useActionsQuery, useEvents } from "@/data/queries";
import { HISTORY_LIMITS } from "@/domain/activity-history";
import { formatRelative, formatTime } from "@/domain/format";
import type { ActionRun, EventLevel } from "@/domain/model";
import { ProgressList } from "@/shared/component-set/progress-list";
import { HostChip } from "@/shared/components/host-chip";
import { NAV_TAB, NAV_TABS_LIST } from "@/shared/components/nav-tabs";
import { EmptyState, QueryError } from "@/shared/components/page";
import { ToneDot } from "@/shared/components/tone";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import { useStore } from "@/shared/lib/store";
import { cn } from "@/shared/lib/utils";
import { Button } from "@/shared/ui/button";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { ScrollArea } from "@/shared/ui/scroll-area";
import { Sheet, SheetContent, SheetDescription, SheetHeader, SheetTitle } from "@/shared/ui/sheet";
import { Skeleton } from "@/shared/ui/skeleton";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";

import { openActionDialog } from "./action-dialog-store";
import { activityStore } from "./activity-store";
import { RunIconTile, StepList } from "./ActionSteps";
import { keepOpenForCoordinatedLayers } from "./layer-coordination";
import { RunOutput } from "./RunOutput";

/** Shared list tones; notifications.css turns the running "accent" into the brand run tone inside the action centre. */
const TONE = { queued: "gray", running: "accent", succeeded: "ok", failed: "err", cancelled: "gray" } as const;
const LEVEL_TONE: Record<EventLevel, "err" | "warn" | "ok" | "info"> = { error: "err", warn: "warn", ok: "ok", info: "info" };

function RunBody({ run, defaultOpen }: { run: ActionRun; defaultOpen: boolean }) {
  const t = useTranslations("activity");
  const [open, setOpen] = useState(defaultOpen);
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="mt-2">
      <CollapsibleTrigger asChild>
        <Button variant="ghost" size="sm" className="h-7 gap-1 px-2 text-xs text-muted-foreground">
          <ChevronDown className={cn("size-3.5 transition-transform", open && "rotate-180")} />
          {t("details")}
        </Button>
      </CollapsibleTrigger>
      <CollapsibleContent className="min-w-0 data-[state=open]:animate-in data-[state=open]:fade-in-0">
        <div className="mt-2 grid min-w-0 grid-cols-[minmax(0,1fr)] gap-3 rounded-xl border bg-background/50 p-3">
          <div className="flex min-w-0 flex-col items-start gap-3 sm:flex-row">
            <div className="min-w-0 flex-1 max-sm:w-full">
              <StepList run={run} />
            </div>
            <Button size="sm" variant="outline" className="h-7 shrink-0 text-xs" onClick={() => openActionDialog(run.id)}>
              <Maximize2 data-icon="inline-start" />
              {t("openDialog")}
            </Button>
          </div>
          {run.services.length > 0 && <p className="text-xs text-muted-foreground">{t("services", { services: run.services.join(", ") })}</p>}
          {run.status === "failed" && run.restorePointId && (
            <div className="flex flex-wrap items-center gap-2 rounded-lg border border-warning/30 bg-warning/10 p-2.5 text-xs text-warning">
              <History className="size-3.5" />
              <span className="min-w-0 flex-1">{t("restoreHint")}</span>
              {run.stackId && (
                <Button asChild size="sm" variant="outline" className="h-7 text-xs">
                  <Link to={`/stacks/${run.stackId}?tab=deployments`} onClick={() => activityStore.set((s) => ({ ...s, open: false }))}>
                    {t("openRestore")}
                  </Link>
                </Button>
              )}
            </div>
          )}
          <RunOutput run={run} defaultOpen={defaultOpen} />
        </div>
      </CollapsibleContent>
    </Collapsible>
  );
}

function ActionsTab({ focusRunId }: { focusRunId?: string }) {
  const t = useTranslations("activity");
  const runs = useActionRuns();
  const query = useActionsQuery();
  const [hideFinished, setHideFinished] = useState(false);
  const shown = hideFinished ? runs.filter((r) => r.status === "running" || r.status === "queued") : runs;

  useEffect(() => {
    if (focusRunId) requestAnimationFrame(() => document.getElementById(`run-${focusRunId}`)?.scrollIntoView({ block: "nearest" }));
  }, [focusRunId]);

  if (query.isError && !runs.length) return <QueryError onRetry={() => void query.refetch()} compact />;
  if (query.isPending && !runs.length) return <Skeleton className="h-24 rounded-xl" />;
  if (!runs.length) return <EmptyState icon={<History />} title={t("emptyActions")} />;
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap items-center justify-between gap-2">
        <HistoryLimit kind="actions" />
        <Button
          variant="outline"
          size="sm"
          className="ml-auto text-xs aria-pressed:border-control/55 aria-pressed:bg-control/15 aria-pressed:text-control-text"
          onClick={() => setHideFinished((v) => !v)}
          aria-pressed={hideFinished}
        >
          {t("clearFinished")}
        </Button>
      </div>
      <ProgressList
        items={shown.map((run) => {
          const Icon = run.status === "queued" ? History : run.status === "running" ? Loader2 : run.status === "succeeded" ? CheckCircle2 : run.status === "cancelled" ? History : XCircle;
          return {
            id: run.id,
            tone: TONE[run.status],
            progress: run.status === "failed" ? Math.max(run.progress, 8) : run.progress,
            highlighted: run.id === focusRunId,
            media: (
              <RunIconTile status={run.status} size="sm">
                <Icon className={cn(run.status === "running" && "animate-spin")} />
              </RunIconTile>
            ),
            title: (
              <span className="flex w-full items-center justify-between gap-2">
                <span className="truncate">{run.title}</span>
                <span className="shrink-0 text-[11px] font-normal text-muted-foreground">{formatTime(run.startedAt)}</span>
              </span>
            ),
            description: (
              <span className={cn(run.status === "failed" && "text-destructive")}>
                {t(`status.${run.status}`)} · {run.target}
                {run.error ? ` · ${run.error}` : ""}
              </span>
            ),
            body: <RunBody run={run} defaultOpen={run.id === focusRunId} />,
          };
        })}
      />
    </div>
  );
}

function EventsTab() {
  const t = useTranslations("activity");
  const events = useEvents();
  const { byId, selection } = useHostScope();
  const [level, setLevel] = useState<"all" | EventLevel>("all");
  const shown = useMemo(
    () => (events.data ?? []).filter((e) => (level === "all" || e.level === level) && (!e.hostId || selection.length === 0 || selection.includes(e.hostId))),
    [events.data, level, selection],
  );
  return (
    <div className="flex flex-col gap-3">
      <HistoryLimit kind="events" />
      <SegmentedToggle<"all" | EventLevel> value={level} onChange={setLevel} label={t("filter.label")} options={(["all", "error", "warn", "ok", "info"] as const).map((l) => ({ value: l, label: t(`filter.${l}`) }))} fit="content" wrap className="self-start" />
      {events.isPending ? (
        <div className="flex flex-col gap-2">
          {Array.from({ length: 6 }, (_, i) => (
            <Skeleton key={i} className="h-12 rounded-lg" />
          ))}
        </div>
      ) : events.isError ? (
        <QueryError onRetry={() => void events.refetch()} compact />
      ) : shown.length === 0 ? (
        <EmptyState icon={<History />} title={t("emptyEvents")} />
      ) : (
        <ol className="flex flex-col">
          {shown.map((e) => (
            <li key={e.id} className="flex gap-3 border-b border-border/50 py-2.5 last:border-0">
              <ToneDot tone={LEVEL_TONE[e.level]} className="mt-1.5" />
              <div className="min-w-0 flex-1">
                <div className="text-sm">{e.title}</div>
                <div className="mt-0.5 flex flex-wrap items-center gap-2 text-xs text-muted-foreground">
                  <span title={formatTime(e.at)}>{formatRelative(e.at)}</span>
                  <span>· {t(`kind.${e.kind}`)}</span>
                  <HostChip host={byId.get(e.hostId ?? "")} />
                  {e.stackId && (
                    <Link to={`/stacks/${e.stackId}`} className="text-primary-text hover:underline" onClick={() => activityStore.set((s) => ({ ...s, open: false }))}>
                      {t("openStack")}
                    </Link>
                  )}
                </div>
              </div>
            </li>
          ))}
        </ol>
      )}
    </div>
  );
}

/** Fixed hub history limits. */
function HistoryLimit({ kind }: { kind: "actions" | "events" }) {
  const t = useTranslations("activity");
  return (
    <p className="flex flex-wrap items-center gap-x-1.5 text-xs text-muted-foreground" data-slot="history-limit">
      {kind === "actions" ? t("historyLimit", { count: HISTORY_LIMITS.actions.default }) : t("historyLimitEvents", { count: HISTORY_LIMITS.events.default })}
    </p>
  );
}

/**
 * Non-modal side sheet: the page, toasts and the action dialog stay usable
 * while it is open. Pointer or focus moving into a toast or the action dialog
 * keeps it open; anywhere else closes it as before.
 */
export function ActivityCenter() {
  const t = useTranslations("activity");
  const ui = useStore(activityStore, (s) => s);
  return (
    <Sheet modal={false} open={ui.open} onOpenChange={(open) => activityStore.set((s) => ({ ...s, open }))}>
      <SheetContent className="w-full gap-0 p-0 shadow-2xl sm:max-w-lg" data-layer="activity" onInteractOutside={keepOpenForCoordinatedLayers}>
        <SheetHeader className="border-b">
          <SheetTitle>{t("title")}</SheetTitle>
          <SheetDescription>{t("description")}</SheetDescription>
        </SheetHeader>
        <Tabs value={ui.tab} onValueChange={(tab) => activityStore.set((s) => ({ ...s, tab: tab as typeof s.tab }))} className="min-h-0 flex-1 gap-0">
          <TabsList aria-label={t("tabsLabel")} className={cn("mx-4 mt-3", NAV_TABS_LIST)}>
            <TabsTrigger value="actions" data-active={ui.tab === "actions"} className={NAV_TAB}>
              <ListChecks aria-hidden /> {t("tabs.actions")}
            </TabsTrigger>
            <TabsTrigger value="events" data-active={ui.tab === "events"} className={NAV_TAB}>
              <History aria-hidden /> {t("tabs.events")}
            </TabsTrigger>
          </TabsList>
          <ScrollArea className="min-h-0 flex-1">
            <div className="p-4">
              <TabsContent value="actions">
                <ActionsTab focusRunId={ui.focusRunId} />
              </TabsContent>
              <TabsContent value="events">
                <EventsTab />
              </TabsContent>
            </div>
          </ScrollArea>
        </Tabs>
      </SheetContent>
    </Sheet>
  );
}
