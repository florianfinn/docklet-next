// Source: docklet-next/issues/611
// Revision: 20b1ec05bcffb69db599d4fa60f0f37ffcbfdb2b
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original M39 #611 one notification for concurrently shown scan runs.

import { X } from "lucide-react";
import { useEffect, type MouseEvent } from "react";
import { useTranslations } from "use-intl";

import { useActionRuns } from "@/data/live";
import type { ActionRun, ActionStatus } from "@/domain/model";

import { ActionDetail } from "./ActionDetail";
import { RunIconTile } from "./ActionSteps";
import { LifeTrack } from "./ActionToastView";
import { RUN_ICON, RUN_TONE } from "./run-phase";
import { classifiedCopy, scanPersistent, useScanCounts } from "./scan-counts";
import { useRunTarget } from "./use-run-target";

const own = (event: MouseEvent) => event.stopPropagation();
const live = (status: ActionStatus) => status === "running" || status === "queued";

/**
 * Aggregate of the members: running while any runs; a failure or partial result keeps the group. A settled
 * member whose detail has not classified its outcome yet holds the countdown: pending is not success.
 */
function groupStatus(members: readonly ActionRun[], persistent: ReadonlySet<string>, classified: ReadonlySet<string>): { status: ActionStatus; persistent: boolean; pending: boolean } {
  if (members.some((r) => live(r.status))) return { status: "running", persistent: false, pending: false };
  if (members.some((r) => r.status === "failed" || persistent.has(r.id))) return { status: "failed", persistent: true, pending: false };
  const status = members.length && members.every((r) => r.status === "cancelled") ? "cancelled" : "succeeded";
  return { status, persistent: false, pending: members.some((r) => !classified.has(r.id)) };
}

type Outcome = (id: string, classified: boolean, persistent: boolean) => void;

function Row({ run, classified, onOpen, onOutcome }: { run: ActionRun; classified: boolean; onOpen: (run: ActionRun) => void; onOutcome: Outcome }) {
  const t = useTranslations("activity"), target = useRunTarget(run), line = useScanCounts(run);
  const persistent = scanPersistent(run), known = classified && !live(run.status);
  useEffect(() => { onOutcome(run.id, known, persistent); }, [run.id, known, persistent, onOutcome]);
  const Icon = RUN_ICON[run.status];
  return (
    <li data-tone={RUN_TONE[run.status]} data-scan-member={run.id} data-status={run.status}>
      <button
        type="button"
        onClick={(e) => { own(e); onOpen(run); }}
        aria-label={t("scanGroup.open", { title: run.title, host: target })}
        className="flex w-full min-w-0 items-start gap-2 rounded-lg px-2 py-1.5 text-left outline-none hover:bg-muted focus-visible:ring-2 focus-visible:ring-ring"
      >
        <span className="mt-0.5 shrink-0 text-(--tone) [&_svg]:size-4"><Icon className={run.status === "running" ? "animate-spin" : undefined} aria-hidden /></span>
        <span className="min-w-0 flex-1">
          <span className="block truncate text-xs font-medium">{target ? `${target} · ${run.title}` : run.title}</span>
          {run.status === "failed" && <span className="block truncate text-xs text-destructive" data-slot="scan-member-error">{run.error ?? t("status.failed")}</span>}
          {(line || run.status !== "failed") && <span className="block truncate text-xs text-muted-foreground" data-slot="scan-member-line">{line ?? t(`status.${run.status}`)}</span>}
        </span>
      </button>
    </li>
  );
}

/**
 * Every visible member keeps its own detail owner, so counts stay live after the Security page unmounts. Its
 * outcome is classified by the terminal detail copy; a failed detail read classifies the listed run (no result).
 */
function Member({ run, onOpen, onOutcome }: { run: ActionRun; onOpen: (run: ActionRun) => void; onOutcome: Outcome }) {
  return <ActionDetail run={run} pending={<Row run={run} classified={false} onOpen={onOpen} onOutcome={onOutcome} />}
    fallback={<Row run={run} classified onOpen={onOpen} onOutcome={onOutcome} />}>{(full, failed) => <Row run={classifiedCopy(run, full, failed)} classified onOpen={onOpen} onOutcome={onOutcome} />}</ActionDetail>;
}

export interface ScanGroupToastProps {
  ids: readonly string[];
  persistent: ReadonlySet<string>;
  classified: ReadonlySet<string>;
  onOutcome: Outcome;
  onOpen: (run: ActionRun) => void;
  onClose: () => void;
}

/** Two or more scan runs shown at once: one card, one row per run, header counting distinct hosts. */
export function ScanGroupToast({ ids, persistent, classified, onOutcome, onOpen, onClose }: ScanGroupToastProps) {
  const t = useTranslations("activity");
  const runs = useActionRuns();
  const members = ids.flatMap((id) => runs.find((r) => r.id === id) ?? []);
  const { status, persistent: kept, pending } = groupStatus(members, persistent, classified);
  const hosts = new Set(members.map((r) => r.hostId ?? "0")).size;
  const Icon = RUN_ICON[status];
  return (
    <div
      data-tone={RUN_TONE[status]}
      data-slot="action-toast"
      data-scan-group
      data-status={status}
      role={status === "failed" ? "alert" : "status"}
      aria-live={status === "failed" ? "assertive" : "polite"}
      aria-label={t("scanGroup.label")}
      className="action-toast w-(--width) max-w-full text-sm"
    >
      <div className="toast-card glass relative flex flex-col gap-2 overflow-hidden rounded-2xl border p-3 shadow-2xl">
        <div className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-3" data-slot="toast-header">
          <RunIconTile status={status} size="md"><Icon className={status === "running" ? "animate-spin" : "animate-pop-in"} /></RunIconTile>
          <div className="min-w-0">
            <p className="truncate text-[15px] leading-snug font-semibold">{t("scanGroup.title", { hosts })}</p>
            <p className="truncate text-xs text-muted-foreground">{t(`status.${status}`)}</p>
          </div>
          <button
            type="button"
            onClick={(e) => { own(e); onClose(); }}
            className="grid size-8 shrink-0 place-items-center rounded-md text-muted-foreground outline-none hover:bg-muted hover:text-foreground focus-visible:ring-2 focus-visible:ring-ring"
            aria-label={t("dismiss")}
          >
            <X className="size-4" />
          </button>
        </div>
        <ul className="flex max-h-64 flex-col overflow-y-auto">
          {members.map((run) => <Member key={run.id} run={run} onOpen={onOpen} onOutcome={onOutcome} />)}
        </ul>
        {status !== "running" && !kept && !pending && <LifeTrack status={status === "cancelled" ? "cancelled" : "succeeded"} onEnd={onClose} />}
      </div>
    </div>
  );
}
