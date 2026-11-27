// Source: docklet-next/issues/682
// Revision: 99c1b0b8c4dfe56182c72749e092b08202436fa5
// Retrieved: 2026-11-27
// License: Apache-2.0
// Deviations: Original combined update web control and progress.

import { useEffect, useRef, useState } from "react";
import { useQueries } from "@tanstack/react-query";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { useHosts } from "@/data/queries";
import { identityLifetime } from "@/data/identity-lifetime";
import { selfUpdateQueryKey, useSelfUpdateCheck } from "@/shared/components/updates/use-self-update-check";
import { useFleetProgress } from "@/shared/components/updates/use-fleet-progress";
import { FleetProgressCard } from "@/shared/components/updates/fleet-progress";
import { useImageError } from "@/features/images/use-image-error";
import { QueryError } from "@/shared/components/page";
import { Button } from "@/shared/ui/button";
import { AlertDialog, AlertDialogContent, AlertDialogTitle, AlertDialogDescription, AlertDialogHeader, AlertDialogFooter, AlertDialogCancel, AlertDialogAction } from "@/shared/ui/alert-dialog";

const dismissedPlans = new WeakMap<object, { version: number; fleetId: string }>();

export function UpdateEverything() {
  const adapter = useAdapter(), hosts = useHosts(), cap = useCapability("selfUpdate", { kind: "hub", id: "0" });
  const progress = useFleetProgress(), { query: hub } = useSelfUpdateCheck("0", { enabled: progress.allowed && cap.available });
  const agents = (hosts.data ?? []).filter(h => h.id !== "0");
  const checks = useQueries({ queries: agents.map(host => ({ queryKey: selfUpdateQueryKey(host.id, host), queryFn: () => adapter.getSelfUpdate(host.id), enabled: progress.allowed && cap.available })) });
  const t = useTranslations("settings.updates.fleet"), single = useTranslations("settings.updates.selfUpdate"), errorText = useImageError();
  const [confirmation, setConfirmation] = useState<{ version: string; hub: boolean }>();
  const [busy, setBusy] = useState(false), [error, setError] = useState<unknown>(), [dismissed, setDismissed] = useState<string | undefined>(() => { const saved = dismissedPlans.get(adapter.subscribeChanges); return saved?.version === progress.version ? saved.fleetId : undefined; });
  const submitting = useRef(false);
  useEffect(() => identityLifetime(adapter).listen(() => { setConfirmation(undefined); setError(undefined); setDismissed(undefined); dismissedPlans.delete(adapter.subscribeChanges); setBusy(false); submitting.current = false; }), [adapter]);
  const availableAgents = checks.filter(q => q.isSuccess && q.data?.eligibility.state === "available").length;
  // With a hub update, agents waiting for the hub follow too.
  const followingAgents = checks.filter(q => q.isSuccess && (q.data?.eligibility.state === "available" || q.data?.eligibility.state === "not-eligible" && q.data.eligibility.code === "update.requires-hub-update")).length;
  // The count is exact only once every agent was read; an unreadable agent is skipped by the plan.
  const agentsRead = hosts.isSuccess && checks.every(q => q.isSuccess || q.isError);
  const state = hub.isSuccess ? hub.data?.eligibility.state : undefined;
  const target = state === "available" ? hub.data?.eligibility.candidate?.version : state === "current" && availableAgents > 0 ? hub.data?.version : undefined;
  const disabled = busy || !agentsRead || !cap.available || !hub.isSuccess || hub.isFetching || !progress.isSuccess || progress.data?.active === true || !!hub.data?.operation && hub.data.operation.phase !== "settled";
  const submit = async () => {
    if (!confirmation || submitting.current || disabled) return;
    const owner = identityLifetime(adapter).version(), version = confirmation.version;
    submitting.current = true; setBusy(true); setError(undefined); setConfirmation(undefined);
    try { await adapter.updateEverything(version); if (owner === identityLifetime(adapter).version()) await progress.refetch(); }
    catch (e) { if (owner === identityLifetime(adapter).version()) setError(e); }
    finally { if (owner === identityLifetime(adapter).version()) { submitting.current = false; setBusy(false); } }
  };
  return <>
    {progress.allowed && target && <Button type="button" variant="outline" disabled={disabled} onClick={() => setConfirmation({ version: target, hub: state === "available" })}>{t("title")}</Button>}
    {error !== undefined && <p role="alert">{errorText(error)}</p>}
    {progress.allowed && progress.isError && <QueryError error={progress.error} onRetry={() => void progress.refetch()} />}
    {progress.data?.fleetId && (progress.data.active || dismissed !== progress.data.fleetId) && <FleetProgressCard status={progress.data} hosts={hosts.data} dismiss={() => { const fleetId = progress.data!.fleetId!; dismissedPlans.set(adapter.subscribeChanges, { version: progress.version, fleetId }); setDismissed(fleetId); }} />}
    <AlertDialog open={!!confirmation} onOpenChange={open => { if (!open) setConfirmation(undefined); }}>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("title")}</AlertDialogTitle><AlertDialogDescription>{confirmation && t(confirmation.hub ? "confirmHub" : "confirmCurrent", { version: confirmation.version, count: confirmation.hub ? followingAgents : availableAgents })}</AlertDialogDescription></AlertDialogHeader>
        <p className="text-sm">{single("noRollback")}</p><p className="text-sm">{single("backupAdvice")} <Link className="underline" to="/hub-backups">{single("backupLink")}</Link></p>
        <AlertDialogFooter><AlertDialogCancel>{single("cancel")}</AlertDialogCancel><AlertDialogAction disabled={disabled} onClick={() => void submit()}>{single("confirm")}</AlertDialogAction></AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  </>;
}
