// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/activity/ActionToasts.tsx
// Revision: d1d32cd1ff7e653ebe5f7fc1ad4151740a091bb2bddbc473680d83f761496ee0
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #69 connected identity and hub settings. #102 identity-scoped toast history and owned-notice cleanup. #102 source-local initial notification history and consumer regression coverage. #126 guarded command mapping and recovery presentation; #110 observed transitions and identifiable current intents. Issue #77: session-bound closure and identity generation cleanup. Simple-user management, assigned-stack access and mode-aware self-service.

import { useIsUser } from "@/data/use-principal";
import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
import { toast } from "sonner";
import { useLocale, useTranslations } from "use-intl";

import { useActionRun } from "@/data/live";
import { renderMessage, renderRun } from "@/data/message-code";
import { useConnectedSettings, useSettings } from "@/data/queries";
import type { CurrentUser } from "@/data/api";
import type { ActionReadHealth } from "@/data/adapter";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { runGateOf, type GateNotice } from "@/data/run-gate";
import { effectivePresentation, toastDetail, uiStyleStore, useUiStyle } from "@/shared/preferences/ui-style-store";

import { actionDialogStore, offerActionDialog, openActionDialog } from "./action-dialog-store";
import { isMajorAction } from "./action-policy";
import { openActivity } from "./activity-store";
import { ActionToastView } from "./ActionToastView";
import { useNow } from "./run-phase";
import { runtimePresentation } from "./runtime-presentation";
import { hasPresentationIntent, presentationStore, RunToastCoordinator } from "./run-presentation";

function ActionToast({ id, toastId }: { id: string; toastId: string | number }) {
  const user = useIsUser();
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
      onOpen={() => (major || user ? openActionDialog(run.id) : openActivity(run.id))}
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
  const client = useQueryClient();
  const lifetime = identityLifetime(adapter);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const t = useTranslations("activity.gate");
  const locale = useLocale();
  const tRef = useRef(t);
  const localeRef = useRef(locale);
  useLayoutEffect(() => {
    tRef.current = t;
    localeRef.current = locale;
  });
  // The hub setting decides for new runs; the design-lab override wins in this browser.
  const user = useIsUser();
  const connected = adapter.kind === "http";
  const demoPresentation = useSettings(!connected).data?.activity.actionPresentation;
  const corePresentation = useConnectedSettings(connected && !user).data?.activity.actionPresentation;
  const hubPresentation = connected ? corePresentation : demoPresentation;
  const hubRef = useRef(hubPresentation);
  useLayoutEffect(() => {
    hubRef.current = hubPresentation;
  });

  useEffect(() => {
    const owned = new Set<string>();
    const clearOwned = () => { for (const id of owned) toast.dismiss(id); owned.clear(); };
    const identity = () => client.getQueryData<CurrentUser>(queryKeys.currentUser)?.id;
    let identityId = identity();
    const createCoordinator = () => new RunToastCoordinator(
      {
        show: (id) => { owned.add(id); toast.custom((toastId) => <ActionToast id={id} toastId={toastId} />, { id, duration: Number.POSITIVE_INFINITY, unstyled: true }); },
        hide: (id) => toast.dismiss(id),
        offerDialog: offerActionDialog,
      },
      {
        autoDialog: () => effectivePresentation(uiStyleStore.get().actionPresentationOverride, hubRef.current) === "dialog",
        dialogRunId: () => actionDialogStore.get().runId,
        state: () => presentationStore.get(),
      },
    );
    let coordinator = createCoordinator();
    let observed = false;
    let health: ActionReadHealth = { failures: [] };
    let initialSources: Set<string> | undefined;
    let seen = new Set<string>();
    const offIdentity = lifetime.listen(() => {
      clearOwned();
      coordinator = createCoordinator(); initialSources = undefined; seen = new Set(); observed = false;
    });
    const offHealth = adapter.subscribeActionReadHealth?.((next) => { health = next; });
    const offRuns = adapter.subscribeActions((runs) => {
      // Identity replacement seeds new history; same-user renewal retains seen/dismissed runs.
      const nextIdentity = identity();
      if (nextIdentity !== identityId) {
        clearOwned();
        coordinator = createCoordinator();
        identityId = nextIdentity;
        initialSources = undefined;
        seen = new Set();
      }
      const failed = new Set(health.failures.flatMap((failure) => failure.hostId === undefined ? [] : [failure.hostId]));
      // Only initially incomplete sources await history; later outages keep ordinary notifications.
      const targetedOnly = runs.length > 0 && runs.every((run) => runtimePresentation(renderRun(run, localeRef.current))?.sourceObserved === false);
      const firstSourceObservation = initialSources === undefined && !targetedOnly;
      const pending = initialSources ?? (targetedOnly ? new Set<string>() : (initialSources = new Set(failed)));
      const rendered = runs.map((run) => renderRun(run, localeRef.current));
      const history = new Set(rendered.filter((run) => (firstSourceObservation || (run.hostId !== undefined && pending.has(run.hostId))) && !seen.has(run.id) && !hasPresentationIntent(run, presentationStore.get())).map((run) => run.id));
      seen = new Set(runs.map((run) => run.id));
      // Health clears before its array: seed this last history first, then retire the source.
      if (!health.failures.some((failure) => failure.hostId === undefined)) {
        for (const hostId of pending) if (!failed.has(hostId)) pending.delete(hostId);
      }
      observed = true;
      coordinator.sync(rendered, history);
    });
    // Presentation changes are not action observations and cannot seed initial history.
    const syncPresentation = () => { if (observed) coordinator.sync(); };
    const offDialog = actionDialogStore.subscribe(syncPresentation);
    const offSurfaces = presentationStore.subscribe(syncPresentation);
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
      offIdentity();
      offRuns();
      offHealth?.();
      clearOwned();
      offDialog();
      offSurfaces();
      offGate?.();
    };
  }, [adapter, client, generation, lifetime]);

  return null;
}
