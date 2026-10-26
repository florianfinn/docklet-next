// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { useUpdateAction } from "./use-update-action";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { Link, useLocation } from "react-router";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import type { UpdatePoint } from "@/domain/stack-updates";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { useImageError } from "@/features/images/use-image-error";
import { UpdateUnavailable } from "./live-state";

export function StackUpdateDialog({ stackId, point, onClose }: { stackId: string; point?: UpdatePoint; onClose: () => void }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const read = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  const write = useCapability(point ? "restoreRestorePoint" : "installStackUpdates", { kind: "stack", id: stackId });
  const points = useCapability("listRestorePoints", { kind: "stack", id: stackId });
  const baseline = useQuery({ queryKey: ["stack", stackId, "update-baseline"], queryFn: () => adapter.getUpdateBaseline!(stackId), enabled: read.available && !!adapter.getUpdateBaseline, staleTime: 0 });
  const action = useUpdateAction(`${stackId}:${baseline.data?.targetIdentity}:${baseline.data?.savedRevision}`);
  const [ack, setAck] = useState<string>(), [receipt, setReceipt] = useState<{ id: string; identity: string }>();
  const data = baseline.data;
  const runId = receipt?.identity === data?.targetIdentity ? receipt?.id : undefined;
  const validPoint = !point || (point.available && !point.orphaned && point.holdState === "none" && point.targetIdentity === data?.targetIdentity);
  const canSend = read.available && write.available && (!point || points.available) && !!data && validPoint && (!point || ack === data?.savedRevision) && !action.busy && !runId;
  const close = () => { action.reset(); onClose(); };
  useEffect(() => identityLifetime(adapter).listen(onClose), [adapter, onClose]);
  return <Dialog open onOpenChange={(open) => { if (!open) close(); }}><DialogContent><DialogHeader><DialogTitle>{t(point ? "restore" : "update")}</DialogTitle><DialogDescription>{t("noData")}</DialogDescription></DialogHeader>
    {!read.available || !write.available || (point && !points.available) ? <UpdateUnavailable reason={read.reason ?? write.reason ?? points.reason} /> : baseline.isError ? <p role="alert">{errorText(baseline.error)}</p> : !data ? <p>{t("loading")}</p> : <>
      {!data.available && <p role="status">{errorText(data.code ?? "update.baseline-unavailable")}</p>}
      {point && <><p>{t("point", { at: point.createdAt })}</p><p>{t(point.sourceState === "partial-source" ? "partial" : "snapshot")}</p><label className="flex items-center gap-2"><Checkbox checked={ack === data.savedRevision} onCheckedChange={(v) => setAck(v === true ? data.savedRevision : undefined)} />{t("replace", { revision: data.savedRevision })}</label></>}
    </>}
    {action.error && <p role="alert">{action.error}</p>}
    {runId && <p role="status">{t("accepted")} <Link to={`/stacks/${encodeURIComponent(stackId)}?tab=deployments`}>{t("openRun", { id: runId })}</Link></p>}
    <div className="flex justify-end gap-2"><Button variant="outline" onClick={close}>{t("close")}</Button><Button disabled={!canSend} onClick={() => void action.run(() => point ? adapter.restoreRestorePoint(stackId, point.id, { expectedRevision: data!.savedRevision, replaceConfiguration: ack === data!.savedRevision }) : adapter.installStackUpdates(stackId, { expectedRevision: data!.savedRevision }), (id) => setReceipt({ id, identity: data!.targetIdentity }))}>{t(point ? "restore" : "update")}</Button></div>
  </DialogContent></Dialog>;
}
export function StackUpdateDialogController() {
  const adapter = useAdapter(), location = useLocation(), [selection, setSelection] = useState<{ stackId: string; location: string }>();
  useEffect(() => { const receive = (event: Event) => { const id = (event as CustomEvent<{ stackId?: unknown }>).detail?.stackId; if (typeof id === "string") setSelection({ stackId: id, location: location.key }); }; window.addEventListener("docklet:stack-update", receive); const off = identityLifetime(adapter).listen(() => setSelection(undefined)); return () => { off(); window.removeEventListener("docklet:stack-update", receive); }; }, [adapter, location.key]);
  const stackId = selection?.location === location.key ? selection.stackId : undefined;
  return stackId ? <StackUpdateDialog key={stackId} stackId={stackId} onClose={() => setSelection(undefined)} /> : null;
}
