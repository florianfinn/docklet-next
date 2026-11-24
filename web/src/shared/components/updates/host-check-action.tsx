// Source: docklet-next/issues/597
// Revision: 6e3c477e97192029f67bcf924da27b3b3849bbd8
// Retrieved: 2026-11-24
// License: Apache-2.0
// Deviations: Original host check feedback from terminal runs and refreshed inventory.

import { CancelledError, useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef } from "react";
import { ScanSearch } from "lucide-react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import type { DockletAdapter } from "@/data/adapter";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useActionRuns } from "@/data/live";
import { useImageError } from "@/features/images/use-image-error";
import { identityLifetime } from "@/data/identity-lifetime";
import { openActionDialog } from "@/features/activity/action-dialog-store";
import { queryKeys } from "@/data/query-keys";
import { AdapterOutcomeError } from "@/data/capability";
import type { CodedActionRun } from "@/data/message-code";
import { hasUpdate, summarizeUpdates } from "@/domain/updates";
import { Button } from "@/shared/ui/button";
import { useUpdateAction } from "./use-update-action";

const active = (run: { status: string }) => run.status === "queued" || run.status === "running";

/** Subscribe before reading: completion during admission or the initial read cannot be lost. */
function waitForUpdateCheck(adapter: DockletAdapter, id: string, signal: AbortSignal): Promise<CodedActionRun> {
  return new Promise((resolve, reject) => {
    const subscription: { stop?: () => void } = {};
    let ended = false;
    const finish = (error?: unknown, run?: CodedActionRun) => {
      if (ended) return;
      ended = true; subscription.stop?.(); signal.removeEventListener("abort", abort);
      if (error) reject(error); else resolve(run!);
    };
    const abort = () => finish(new CancelledError());
    const observe = (runs: CodedActionRun[]) => {
      const run = runs.find((r) => r.id === id || r.jobRef?.runId === id);
      if (!run || active(run)) return;
      finish(undefined, run);
    };
    if (signal.aborted) { abort(); return; }
    signal.addEventListener("abort", abort, { once: true });
    subscription.stop = adapter.subscribeActions(observe);
    if (ended) subscription.stop();
    else void adapter.listActions().then(observe, finish);
  });
}

export function HostCheckAction({ hostId }: { hostId?: string }) {
  const adapter = useAdapter(), client = useQueryClient(), t = useTranslations("stacks.hostCheck");
  const errors = useImageError();
  const unfinished = useTranslations("containers.updateCheck")("unfinished");
  const capability = useCapability("checkAllUpdates"), action = useUpdateAction(hostId ?? "all", unfinished), runs = useActionRuns();
  const pending = useRef<AbortController | undefined>(undefined);
  useEffect(() => {
    const cancel = () => pending.current?.abort();
    const off = identityLifetime(adapter).listen(cancel);
    return () => { cancel(); off(); };
  }, [adapter, hostId]);
  // A running all-hosts check also covers each host.
  const allHosts = (r: { jobRef?: unknown; stackId?: string; containerId?: string }) => !r.jobRef && !r.stackId && !r.containerId;
  const observedBusy = runs.some((r) => active(r) && r.kind === "check-updates" && (allHosts(r) || hostId !== undefined && r.hostId === hostId && (r.jobRef?.jobId === "update-check-host" || adapter.kind === "demo" && !r.stackId && !r.containerId)));
  const busy = action.busy || observedBusy;
  const supported = !!adapter.checkHostUpdates;
  const check = () => {
    pending.current?.abort();
    const controller = new AbortController(); pending.current = controller;
    return action.run(async () => {
      const id = await adapter.checkHostUpdates!(hostId);
      const run = await waitForUpdateCheck(adapter, id, controller.signal);
      if (controller.signal.aborted) throw new CancelledError();
      // Unfiltered reads count all of this host, including rows hidden in the overview.
      const [stacks, containers] = await Promise.all([adapter.listStacks(), adapter.listContainers()]);
      if (controller.signal.aborted) throw new CancelledError();
      // A partial run still reports its counts; the failed target count is shown when the run's targets can be read (D-7).
      const failed = run.partial && run.jobRef && adapter.getUpdateJobTargets
        ? await adapter.getUpdateJobTargets(run.hostId ?? "0", run.jobRef.jobId, run.jobRef.runId).then((targets) => targets.filter((target) => target.status === "failed").length, () => undefined)
        : undefined;
      if (controller.signal.aborted) throw new CancelledError();
      return { run, stacks, containers, failed };
    }, ({ run, stacks, containers, failed }) => {
      client.setQueryData(queryKeys.stacks, stacks); client.setQueryData(queryKeys.containers, containers);
      if (run.status !== "succeeded" && !run.partial) {
        toast.error(errors(new AdapterOutcomeError({ kind: "error", code: run.error?.code ?? "update.check-unavailable" })));
        return;
      }
      const result = t("result", {
        stacks: stacks.filter((s) => (hostId === undefined || s.hostId === hostId) && summarizeUpdates(s.services).pending > 0).length,
        containers: containers.filter((c) => (hostId === undefined || c.hostId === hostId) && !c.stackId && hasUpdate(c.update)).length,
      });
      if (run.status === "succeeded") { toast.success(result); return; }
      toast.warning(result, { description: failed ? t("partialCount", { count: failed }) : t("partial"), action: { label: t("openRun"), onClick: () => openActionDialog(run.id) } });
    });
  };
  return <div className="flex min-w-0 flex-col gap-1">
    <Button type="button" size="sm" variant="outline" disabled={!supported || !capability.available || busy} title={capability.reason} aria-busy={busy} onClick={() => void check()}>
      <ScanSearch data-icon="inline-start" className={busy ? "animate-pulse" : undefined} />{t(busy ? "checking" : hostId === undefined ? "all" : "check")}
    </Button>
    {action.error && <p role="alert" className="max-w-sm text-xs">{action.error}</p>}
  </div>;
}
