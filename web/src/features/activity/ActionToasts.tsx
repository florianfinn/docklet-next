import { useEffect, useLayoutEffect, useRef } from "react";
import { toast } from "sonner";
import { useLocale, useTranslations } from "use-intl";

import { useActionRun } from "@/data/live";
import { renderMessage, renderRun } from "@/data/message-code";
import { useConnectedSettings, useSettings } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { runGateOf, type GateNotice } from "@/data/run-gate";
import { effectivePresentation, toastDetail, uiStyleStore, useUiStyle } from "@/shared/preferences/ui-style-store";

import { actionDialogStore, offerActionDialog, openActionDialog } from "./action-dialog-store";
import { isMajorAction } from "./action-policy";
import { openActivity } from "./activity-store";
import { ActionToastView } from "./ActionToastView";
import { useNow } from "./run-phase";
import { presentationStore, RunToastCoordinator } from "./run-presentation";

function ActionToast({ id, toastId }: { id: string; toastId: string | number }) {
  const run = useActionRun(id);
  const scale = useUiStyle("toastScale");
  const design = useUiStyle("toastDesign");
  const now = useNow(run?.status === "running");
  if (!run) return null;
  const major = isMajorAction(run);
  return (
    <ActionToastView
      run={run}
      design={design}
      detail={toastDetail(scale)}
      now={now}
      major={major}
      onClose={() => toast.dismiss(toastId)}
      onOpen={() => (major ? openActionDialog(run.id) : openActivity(run.id))}
    />
  );
}

/**
 * Shows one progressive toast per action unless the action dialog or an
 * inline surface already shows that run (see RunToastCoordinator). Major
 * actions offer the dialog according to their start's presentation; it never
 * opens the action centre by itself.
 */
export function ActionToasts() {
  const adapter = useAdapter();
  const t = useTranslations("activity.gate");
  const locale = useLocale();
  const tRef = useRef(t);
  const localeRef = useRef(locale);
  useLayoutEffect(() => {
    tRef.current = t;
    localeRef.current = locale;
  });
  // The hub setting decides for new runs; the design-lab override wins in this browser.
  const connected = adapter.kind === "http";
  const demoPresentation = useSettings(!connected).data?.activity.actionPresentation;
  const corePresentation = useConnectedSettings(connected).data?.activity.actionPresentation;
  const hubPresentation = connected ? corePresentation : demoPresentation;
  const hubRef = useRef(hubPresentation);
  useLayoutEffect(() => {
    hubRef.current = hubPresentation;
  });

  useEffect(() => {
    const coordinator = new RunToastCoordinator(
      {
        show: (id) => toast.custom((toastId) => <ActionToast id={id} toastId={toastId} />, { id, duration: Number.POSITIVE_INFINITY, unstyled: true }),
        hide: (id) => toast.dismiss(id),
        offerDialog: offerActionDialog,
      },
      {
        autoDialog: () => effectivePresentation(uiStyleStore.get().actionPresentationOverride, hubRef.current) === "dialog",
        dialogRunId: () => actionDialogStore.get().runId,
        state: () => presentationStore.get(),
      },
    );
    const offRuns = adapter.subscribeActions((runs) => coordinator.sync(runs.map((run) => renderRun(run, localeRef.current))));
    const offDialog = actionDialogStore.subscribe(() => coordinator.sync());
    const offSurfaces = presentationStore.subscribe(() => coordinator.sync());
    // One notice per target, replaced on every repeat (stable toast id), never a new run toast.
    const offGate = runGateOf(adapter)?.subscribe((n: GateNotice) => {
      const tr = tRef.current;
      const title = n.title && renderMessage(n.title, localeRef.current);
      if (n.reason === "inflight") {
        if (n.runId) coordinator.reveal(n.runId);
      } else if (n.reason === "busy") {
        toast.warning(title ? tr("busy", { title }) : tr("busyUnknown"), { id: `gate:${n.key}` });
      } else {
        const seconds = n.finishedAt ? Math.max(1, Math.round((Date.now() - Date.parse(n.finishedAt)) / 1000)) : 0;
        const retry = n.retry;
        toast.info(title && seconds ? tr("fresh", { title, seconds }) : tr("freshUnknown"), {
          id: `gate:${n.key}`,
          action: retry ? { label: tr("recheck"), onClick: () => void retry().catch(() => undefined) } : undefined,
        });
      }
    });
    return () => {
      offRuns();
      offDialog();
      offSurfaces();
      offGate?.();
    };
  }, [adapter]);

  return null;
}
