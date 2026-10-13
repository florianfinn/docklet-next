import { CheckCircle2, ChevronRight, History, Loader2, RotateCcw, XCircle } from "lucide-react";
import { useRef, useState } from "react";
import { useLocale, useTranslations } from "use-intl";

import { localizeRun } from "@/data/demo/demo-text";
import { useActionRuns } from "@/data/live";
import { formatDateTime, formatRelative } from "@/domain/format";
import type { ActionRun, RestorePoint, StackDetail } from "@/domain/model";
import { versionLabel } from "@/domain/updates";
import { StepList } from "@/features/activity/ActionSteps";
import { deployRuns, shownRun, type HistoryPick } from "@/features/activity/deploy-history";
import { RUN_TONE, useNow } from "@/features/activity/run-phase";
import { startPresented, useInlineRunSurface } from "@/features/activity/run-presentation";
import { RunFocusView } from "@/features/activity/RunFocus";
import { EmptyState } from "@/shared/components/page";
import { IconTile, ToneBadge } from "@/shared/components/tone";
import { useDemoText } from "@/shared/hooks/use-demo-text";
import { cn } from "@/shared/lib/utils";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle } from "@/shared/ui/alert-dialog";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/shared/ui/card";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";

import { useStackCommands } from "@/shared/hooks/use-stack-commands";

const TONE = RUN_TONE;
const ICON = { running: Loader2, succeeded: CheckCircle2, failed: XCircle } as const;

const durationOf = (run: ActionRun) => (run.finishedAt ? Math.round((new Date(run.finishedAt).getTime() - new Date(run.startedAt).getTime()) / 1000) : 0);

/** The one run in the main view, in the same focus design as the action dialog, with its open raw output. */
function RunCard({ run }: { run: ActionRun }) {
  const t = useTranslations("stacks.deployments");
  const now = useNow(run.status === "running");
  return (
    <article data-tone={RUN_TONE[run.status]} data-slot="deployment-current" data-run={run.id} className={cn("rounded-xl border bg-card/60 p-4", run.status === "running" && "border-(--tone-edge)/50 shadow-lg shadow-primary/10")}>
      <div className="mb-4 flex flex-wrap items-center gap-2">
        <span className="font-medium">{run.title}</span>
        <ToneBadge tone="gray">{t(`trigger.${run.kind as "deploy"}`)}</ToneBadge>
        <span className="ml-auto text-xs text-muted-foreground" title={formatDateTime(run.startedAt)}>
          {run.status === "running" ? t("running") : `${formatRelative(run.startedAt)} · ${t("duration", { seconds: durationOf(run) })}`}
        </span>
      </div>
      <RunFocusView run={run} now={now} outputOpen />
      {run.status === "failed" && run.restorePointId && <p className="mt-3 text-xs text-muted-foreground">{t("failedHint")}</p>}
      <Collapsible className="mt-3">
        <CollapsibleTrigger className="group/steps flex items-center gap-1.5 rounded-md text-xs font-medium text-muted-foreground outline-none hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring">
          <ChevronRight className="size-3.5 transition-transform duration-(--dur-fast) group-data-[state=open]/steps:rotate-90" aria-hidden />
          {t("steps")}
        </CollapsibleTrigger>
        <CollapsibleContent className="pt-3">
          <StepList run={run} />
        </CollapsibleContent>
      </Collapsible>
    </article>
  );
}

/** Collapsed list of all earlier runs; picking one shows it in the main view, nothing is deleted. */
function HistoryList({ runs, shownId, onPick }: { runs: ActionRun[]; shownId: string; onPick: (id: string) => void }) {
  const t = useTranslations("stacks.deployments");
  const ta = useTranslations("activity.history");
  const [open, setOpen] = useState(false);
  if (!runs.length) return null;
  return (
    <Collapsible open={open} onOpenChange={setOpen} className="mt-3 rounded-xl border bg-card/40" data-slot="deployment-history">
      <CollapsibleTrigger className="group/hist flex w-full items-center gap-2 rounded-xl px-4 py-3 text-left text-sm font-medium outline-none hover:bg-muted/30 focus-visible:ring-2 focus-visible:ring-ring">
        <ChevronRight className="size-4 text-muted-foreground transition-transform duration-(--dur-fast) group-data-[state=open]/hist:rotate-90" aria-hidden />
        <History className="size-4 text-muted-foreground" aria-hidden />
        {ta("older", { count: runs.length })}
      </CollapsibleTrigger>
      <CollapsibleContent>
        <p className="px-4 pb-2 text-xs text-muted-foreground">{ta("olderHint")}</p>
        <ol className="flex flex-col border-t">
          {runs.map((r) => {
            const Icon = ICON[r.status];
            const shown = r.id === shownId;
            return (
              <li key={r.id} data-tone={TONE[r.status]} className="border-b last:border-0">
                <button
                  type="button"
                  onClick={() => onPick(r.id)}
                  aria-current={shown || undefined}
                  aria-label={ta("show", { title: r.title })}
                  className={cn("flex w-full items-center gap-3 px-4 py-2.5 text-left text-sm outline-none hover:bg-muted/30 focus-visible:bg-muted/40 focus-visible:ring-2 focus-visible:ring-ring focus-visible:ring-inset", shown && "bg-primary/8")}
                >
                  <Icon className={cn("size-4 shrink-0 text-(--tone)", r.status === "running" && "animate-spin")} aria-hidden />
                  <span className="min-w-0 flex-1 truncate">{r.title}</span>
                  <ToneBadge tone="gray" className="hidden sm:inline-flex">
                    {t(`trigger.${r.kind as "deploy"}`)}
                  </ToneBadge>
                  {shown && <ToneBadge tone="violet">{ta("shown")}</ToneBadge>}
                  <span className="shrink-0 text-xs text-muted-foreground tabular-nums" title={formatDateTime(r.startedAt)}>
                    {r.status === "running" ? t("running") : `${formatRelative(r.startedAt)} · ${t("duration", { seconds: durationOf(r) })}`}
                  </span>
                </button>
              </li>
            );
          })}
        </ol>
      </CollapsibleContent>
    </Collapsible>
  );
}

function RestoreCard({ point, onRestore, busy }: { point: RestorePoint; onRestore: () => void; busy: boolean }) {
  const dt = useDemoText();
  const t = useTranslations("stacks.deployments");
  return (
    <li className="flex flex-col gap-2 rounded-xl border bg-muted/20 p-3">
      <div className="flex items-center gap-2">
        <History className="size-4 text-warning" aria-hidden />
        <span className="flex-1 text-sm font-medium">{dt(point.reason)}</span>
        <span className="text-xs text-muted-foreground">{formatRelative(point.createdAt)}</span>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {point.images.map((i) => (
          <ToneBadge key={i.service} tone="gray" className="font-mono">
            {i.service} {versionLabel(i.image)}
          </ToneBadge>
        ))}
        <ToneBadge tone="gray" className="font-mono">
          {t("compose")}
        </ToneBadge>
      </div>
      <Button size="sm" variant="outline" className="self-start" onClick={onRestore} disabled={busy}>
        <RotateCcw data-icon="inline-start" /> {t("restore")}
      </Button>
    </li>
  );
}

/**
 * Deploy history: one stable main view with the newest run (or a run picked
 * from the history) and its raw output; earlier runs sit in a collapsed,
 * complete history. A new run takes over the main view. While the tab is
 * mounted it is the inline surface for runs started here, so they get neither
 * the action dialog nor a toast until the tab is left.
 */
export function DeploymentsTab({ stack }: { stack: StackDetail }) {
  const t = useTranslations("stacks.deployments");
  const ta = useTranslations("activity.history");
  const tc = useTranslations("common.actions");
  const cmd = useStackCommands();
  const live = useActionRuns();
  const dt = useDemoText();
  const locale = useLocale();
  const [confirm, setConfirm] = useState<RestorePoint | null>(null);
  const [pick, setPick] = useState<HistoryPick | null>(null);
  const mainRef = useRef<HTMLHeadingElement>(null);
  const runs = deployRuns(live, stack.deployments.map((r) => localizeRun(r, locale)), stack.id);
  const latest = runs[0];
  const shown = shownRun(runs, pick);
  const points = [...stack.restorePoints].sort((a, b) => b.createdAt.localeCompare(a.createdAt));
  useInlineRunSurface(stack.id);

  return (
    <div className="grid gap-4 lg:grid-cols-[minmax(0,1fr)_minmax(19rem,25rem)]" data-slot="deployments">
      <section aria-labelledby="dep-title">
        <h2 id="dep-title" className="mb-3 text-sm font-medium text-muted-foreground">
          {t("title")}
        </h2>
        {shown && latest ? (
          <>
            <div className="mb-2 flex min-h-8 flex-wrap items-center gap-2">
              <h3 ref={mainRef} tabIndex={-1} className="text-xs font-semibold tracking-wide text-muted-foreground uppercase outline-none" data-slot="deployment-view">
                {shown.id === latest.id ? ta("current") : ta("picked")}
              </h3>
              {shown.id !== latest.id && (
                <Button size="sm" variant="outline" className="ml-auto h-7 text-xs" onClick={() => setPick(null)}>
                  {ta("showLatest")}
                </Button>
              )}
            </div>
            <RunCard key={shown.id} run={shown} />
            <HistoryList
              runs={runs.slice(1)}
              shownId={shown.id}
              onPick={(runId) => {
                setPick({ runId, latestId: latest.id });
                requestAnimationFrame(() => mainRef.current?.focus());
              }}
            />
          </>
        ) : (
          <EmptyState icon={<History />} title={t("empty")} />
        )}
      </section>
      {/* Same height as the history column, so both panels end on one line. */}
      <Card className="gap-3 self-stretch py-4 lg:mt-8" data-slot="restore-points">
        <CardHeader className="px-4">
          <CardTitle className="flex items-center gap-2.5">
            <IconTile tone="warn" size="sm">
              <History />
            </IconTile>
            {t("restoreTitle")}
          </CardTitle>
          <CardDescription>{t("restoreHint")}</CardDescription>
        </CardHeader>
        <CardContent className="px-4">
          {points.length ? (
            <ul className="flex flex-col gap-2">
              {points.map((p) => (
                <RestoreCard key={p.id} point={p} busy={!!stack.busyActionId} onRestore={() => setConfirm(p)} />
              ))}
            </ul>
          ) : (
            <p className="text-sm text-muted-foreground">{t("restoreNone")}</p>
          )}
        </CardContent>
      </Card>
      <AlertDialog open={!!confirm} onOpenChange={(o) => !o && setConfirm(null)}>
        <AlertDialogContent>
          <AlertDialogHeader>
            <AlertDialogTitle>{t("restoreConfirmTitle")}</AlertDialogTitle>
            <AlertDialogDescription>{t("restoreConfirmBody", { reason: dt(confirm?.reason ?? "") })}</AlertDialogDescription>
          </AlertDialogHeader>
          <AlertDialogFooter>
            <AlertDialogCancel>{tc("cancel")}</AlertDialogCancel>
            <AlertDialogAction
              onClick={() => {
                if (confirm) void startPresented({ stackId: stack.id }, "inline", () => cmd.restore(stack.id, confirm.id));
                setConfirm(null);
              }}
            >
              {t("restoreConfirm")}
            </AlertDialogAction>
          </AlertDialogFooter>
        </AlertDialogContent>
      </AlertDialog>
    </div>
  );
}
