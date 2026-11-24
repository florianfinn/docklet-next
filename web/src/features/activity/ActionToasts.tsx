// Source: approved Docklet design prototype (maintainer's own work)/react-opus/src/features/activity/ActionToasts.tsx
// Revision: d1d32cd1ff7e653ebe5f7fc1ad4151740a091bb2bddbc473680d83f761496ee0
// Retrieved: 2026-10-13
// License: Apache-2.0
// Deviations: Adapter port cleanup, demo-only options and coded run/event localization after the historical import. See web/PROVENANCE.md. #69 connected identity and hub settings. #102 identity-scoped toast history and owned-notice cleanup. #102 source-local initial notification history and consumer regression coverage. #126 guarded command mapping and recovery presentation; #110 observed transitions and identifiable current intents. Issue #77: session-bound closure and identity generation cleanup. Simple-user management, assigned-stack access and mode-aware self-service. #370 agent restarts notify on first observation.

import { useIsUser } from "@/data/use-principal";
import { useQueryClient } from "@tanstack/react-query";
import { useCallback, useEffect, useLayoutEffect, useRef, useSyncExternalStore } from "react";
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
import { ActionDetail } from "./ActionDetail";
import { isScanRun } from "./scan-counts";
import { ScanGroupToast } from "./ScanGroupToast";
import { createStore, useStore, type Store } from "@/shared/lib/store";
import type { ActionRun } from "@/domain/model";
import { useNow } from "./run-phase";
import { runtimePresentation } from "./runtime-presentation";
import { hasPresentationIntent, notifiesOnFirstObservation, presentationStore, RunToastCoordinator } from "./run-presentation";

function ActionToast({ id, toastId, onClose }: { id: string; toastId: string | number; onClose?: () => void }) {
  const user = useIsUser();
  const run = useActionRun(id);
  const scale = useUiStyle("toastScale");
  const design = useUiStyle("toastDesign");
  const now = useNow(run?.status === "running");
  if (!run) return null;
  const major = isMajorAction(run);
  const view = (observed: ActionRun) => (
    <ActionToastView
      run={observed}
      design={design}
      detail={toastDetail(scale)}
      now={now}
      major={major}
      opensDialog={user}
      onClose={onClose ?? (() => toast.dismiss(toastId))}
      onOpen={() => (major || user ? openActionDialog(run.id) : openActivity(run.id))}
    />
  );
  // Visible scan toasts own their counters even after the Security page unmounts.
  return run.kind === "scan" || run.sourceOperation === "vulnerability.candidate-scan"
    ? <ActionDetail run={run} fallback={view(run)}>{view}</ActionDetail> : view(run);
}

const SCAN_GROUP = "scan-group";
interface ScanGroupState { ids: readonly string[]; persistent: ReadonlySet<string> }

/** One sonner toast for every scan run the coordinator shows; a single member keeps the ordinary run toast. */
function ScanGroup({ store, close }: { store: Store<ScanGroupState>; close: () => void }) {
  const user = useIsUser();
  const state = useStore(store, (s) => s);
  const onPersistent = useCallback((id: string, value: boolean) => store.set((s) => {
    if (s.persistent.has(id) === value || !s.ids.includes(id)) return s;
    const persistent = new Set(s.persistent);
    if (value) persistent.add(id); else persistent.delete(id);
    return { ...s, persistent };
  }), [store]);
  // A closed group renders nothing during its exit, so no stale countdown can close a newer group.
  if (!state.ids.length) return null;
  if (state.ids.length === 1) return <ActionToast id={state.ids[0]!} toastId={SCAN_GROUP} onClose={close} />;
  return <ScanGroupToast ids={state.ids} persistent={state.persistent} onPersistent={onPersistent} onClose={close}
    onOpen={(run) => (user ? openActionDialog(run.id) : openActivity(run.id))} />;
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
    // Scan runs share one toast (#611). A new epoch retires dismiss callbacks of an earlier group toast.
    const group = createStore<ScanGroupState>({ ids: [], persistent: new Set() });
    let epoch = 0, latest: ActionRun[] = [];
    const renderGroup = () => {
      if (!group.get().ids.length) { epoch++; toast.dismiss(SCAN_GROUP); return; }
      const mine = epoch;
      const forget = () => { if (mine === epoch) { epoch++; group.set({ ids: [], persistent: new Set() }); } };
      const close = () => { forget(); toast.dismiss(SCAN_GROUP); };
      toast.custom(() => <ScanGroup store={group} close={close} />, { id: SCAN_GROUP, duration: Number.POSITIVE_INFINITY, unstyled: true, onDismiss: forget });
    };
    const setMembers = (change: (ids: readonly string[]) => readonly string[]) => {
      const before = group.get().ids, ids = change(before);
      if (ids === before) return;
      group.set((s) => ({ ids, persistent: new Set([...s.persistent].filter((id) => ids.includes(id))) }));
      renderGroup();
    };
    const isScan = (id: string) => { const run = latest.find((r) => r.id === id); return run !== undefined && isScanRun(run); };
    const clearOwned = () => {
      for (const id of owned) toast.dismiss(id); owned.clear();
      if (group.get().ids.length) { group.set({ ids: [], persistent: new Set() }); renderGroup(); }
    };
    const identity = () => client.getQueryData<CurrentUser>(queryKeys.currentUser)?.id;
    let identityId = identity();
    const createCoordinator = () => new RunToastCoordinator(
      {
        show: (id) => {
          if (isScan(id)) { setMembers((ids) => (ids.includes(id) ? ids : [...ids, id])); return; }
          owned.add(id); toast.custom((toastId) => <ActionToast id={id} toastId={toastId} />, { id, duration: Number.POSITIVE_INFINITY, unstyled: true });
        },
        hide: (id) => { if (group.get().ids.includes(id)) setMembers((ids) => ids.filter((m) => m !== id)); else toast.dismiss(id); },
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
      const history = new Set(rendered.filter((run) => (firstSourceObservation || (run.hostId !== undefined && pending.has(run.hostId))) && !seen.has(run.id) && !hasPresentationIntent(run, presentationStore.get()) && !notifiesOnFirstObservation(run)).map((run) => run.id));
      seen = new Set(runs.map((run) => run.id));
      // Health clears before its array: seed this last history first, then retire the source.
      if (!health.failures.some((failure) => failure.hostId === undefined)) {
        for (const hostId of pending) if (!failed.has(hostId)) pending.delete(hostId);
      }
      observed = true;
      latest = rendered;
      coordinator.sync(rendered, history);
      // The coordinator forgets runs that left the list without a hide; the group drops them too.
      setMembers((ids) => (ids.every((id) => rendered.some((r) => r.id === id)) ? ids : ids.filter((id) => rendered.some((r) => r.id === id))));
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
