// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231. M24 S6: observed recovery and update UI follow-ups. M34 #546: one blocking reason and a deploy action. M35 #555: close on admission, revision-bound restore without hash text.

import { COMMAND_DIALOG_CONTENT } from "@/shared/components/dialog-layout";
import { formatDateTime } from "@/domain/format";

import { useStack } from "@/data/queries";
import { AdapterOutcomeError } from "@/data/capability";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";
import { BindingRefusal } from "./binding-refusal";
import { StackBindingNotice, blockingBindings, useStackEligibility } from "./stack-binding-notice";
import { useUpdateAction } from "./use-update-action";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useLocation } from "react-router";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import type { UpdatePoint } from "@/domain/stack-updates";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { useImageError } from "@/features/images/use-image-error";
import { UpdateUnavailable } from "./live-state";

export function StackUpdateDialog({ stackId, point, services: initialServices, autoRestore = false, onClose }: { stackId: string; point?: UpdatePoint; services?: string[]; autoRestore?: boolean; onClose: () => void }) {
  const stack = useStack(stackId);
  const [service, setService] = useState(initialServices?.[0] ?? "*"), [wholeStackHint, setWholeStackHint] = useState(false);
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), errorText = useImageError();
  const read = useCapability("getUpdateBaseline", { kind: "stack", id: stackId });
  const write = useCapability(point ? "restoreRestorePoint" : "installStackUpdates", { kind: "stack", id: stackId });
  const points = useCapability("listRestorePoints", { kind: "stack", id: stackId });
  const redeploy = useCapability("redeployStack", { kind: "stack", id: stackId }), commands = useStackCommands();
  const baseline = useQuery({ queryKey: ["stack", stackId, "update-baseline"], queryFn: () => adapter.getUpdateBaseline!(stackId), enabled: read.available && !!adapter.getUpdateBaseline, staleTime: 0 });
  const action = useUpdateAction(`${stackId}:${baseline.data?.targetIdentity}:${baseline.data?.savedRevision}:${service}`);
  const data = baseline.data;
  const eligibility = useStackEligibility(stackId, !point);
  const bindingBlocked = !point && blockingBindings(eligibility.bindings, service, eligibility.updatable).length > 0;
  // Mutations bind the current, successfully read revision; a status without one is shown only.
  const revisionReady = !baseline.isError && /^[a-f0-9]{64}$/.test(data?.savedRevision ?? "");
  // Consent belongs to one target revision and is dropped whenever that changes or becomes unusable.
  const ackKey = revisionReady ? `${data!.targetIdentity}:${data!.savedRevision}` : undefined;
  const [ack, setAck] = useState(false), [ackFor, setAckFor] = useState(ackKey);
  if (ackFor !== ackKey) { setAckFor(ackKey); setAck(false); }
  const validPoint = !point || (point.available && !point.orphaned && point.holdState === "none" && point.targetIdentity === data?.targetIdentity);
  // An update needs a verified baseline; a restore point carries its own configuration.
  const blocked = !point && !!data && !data.available;
  const canSend = !bindingBlocked && !eligibility.checking && read.available && write.available && (!point || points.available) && !!data && !blocked && revisionReady && validPoint && (!point || ack) && !action.busy;
  const close = () => { action.reset(); onClose(); };
  useEffect(() => identityLifetime(adapter).listen(onClose), [adapter, onClose]);
  return <Dialog open onOpenChange={(open) => { if (!open) close(); }}><DialogContent className={COMMAND_DIALOG_CONTENT}><DialogHeader><DialogTitle>{t(point ? "restore" : "update")}</DialogTitle><DialogDescription>{t(autoRestore ? "updateWithCheckHelp" : "noData")}</DialogDescription></DialogHeader>
    {!read.available || !write.available || (point && !points.available) ? <UpdateUnavailable reason={read.reason ?? write.reason ?? points.reason} /> : baseline.isError ? <p role="alert">{errorText(baseline.error)}</p> : !data ? <p>{t("loading")}</p> : <>
      {!data.available && <p role="status">{errorText(data.code ?? "update.baseline-unavailable")}</p>}
      {point && <><p>{t("point", { at: formatDateTime(point.createdAt) })}</p><p>{t(point.sourceState === "partial-source" ? "partial" : "snapshot")}</p>{revisionReady ? <label className="flex min-w-0 items-start gap-2"><Checkbox checked={ack} onCheckedChange={(v) => setAck(v === true)} /><span className="min-w-0 flex-1">{t("replace")}</span></label> : <p role="status">{t("restoreNeedsRevision")}</p>}</>}
    </>}
    {!point && !blocked && <Select value={service} onValueChange={(v) => { setService(v); setWholeStackHint(false); action.reset(); }}><SelectTrigger aria-label={t("update")}><SelectValue /></SelectTrigger><SelectContent><SelectGroup><SelectItem value="*">{t("update")}</SelectItem>{(stack.data?.services ?? []).map((s) => <SelectItem key={s.name} value={s.name}>{t("service", { name: s.name })}</SelectItem>)}</SelectGroup></SelectContent></Select>}
    {wholeStackHint && <p role="status">{t("wholeStack")}</p>}
    {!point && <StackBindingNotice stackId={stackId} />}
    {action.binding && <BindingRefusal binding={action.binding} />}
    {action.error && <p role="alert">{action.error}</p>}
    <div className="flex min-w-0 flex-col-reverse gap-2 sm:flex-row sm:justify-end"><Button variant="outline" onClick={close}>{t("close")}</Button>{blocked ? redeploy.available && <Button onClick={() => { void commands.run(stackId, "redeploy"); close(); }}>{t("deployNow")}</Button> : <Button disabled={!canSend} onClick={() => canSend && void action.run(() => point ? adapter.restoreRestorePoint(stackId, point.id, { expectedRevision: data!.savedRevision, replaceConfiguration: ack }) : adapter.installStackUpdates(stackId, { expectedRevision: data!.savedRevision, ...(autoRestore ? { autoRestore: true } : {}), ...(service === "*" ? {} : { services: [service] }) }), () => close(), (error) => setWholeStackHint(error instanceof AdapterOutcomeError && error.outcome.kind === "error" && ["update.service-coupled", "update.image-binding-unsupported"].includes(error.outcome.code)))}>{t(point ? "restore" : "update")}</Button>}</div>
  </DialogContent></Dialog>;
}
export function StackUpdateDialogController() {
  const adapter = useAdapter(), location = useLocation(), [selection, setSelection] = useState<{ stackId: string; services?: string[]; autoRestore?: boolean; location: string }>();
  useEffect(() => { const receive = (event: Event) => { const id = (event as CustomEvent<{ stackId?: unknown; services?: string[]; autoRestore?: boolean }>).detail?.stackId; if (typeof id === "string") setSelection({ stackId: id, services: (event as CustomEvent<{ services?: string[]; autoRestore?: boolean }>).detail.services, autoRestore: (event as CustomEvent<{ autoRestore?: boolean }>).detail.autoRestore === true, location: location.key }); }; window.addEventListener("docklet:stack-update", receive); const off = identityLifetime(adapter).listen(() => setSelection(undefined)); return () => { off(); window.removeEventListener("docklet:stack-update", receive); }; }, [adapter, location.key]);
  const stackId = selection?.location === location.key ? selection.stackId : undefined;
  return stackId ? <StackUpdateDialog key={`${stackId}:${selection?.services?.join(",") ?? "*"}:${selection?.autoRestore === true}`} stackId={stackId} services={selection?.services} autoRestore={selection?.autoRestore} onClose={() => setSelection(undefined)} /> : null;
}
