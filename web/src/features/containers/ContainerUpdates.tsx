// Source: docklet-next/issues/342
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original standalone update and previous-image recovery gestures.

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { isSystemWorkload } from "@/domain/system";
import type { ContainerInfo } from "@/domain/model";
import { formatDateTime } from "@/domain/format";
import { hasUpdate } from "@/domain/updates";
import { inventoryId } from "@/data/http/inventory-mapping";
import { useHubTimeZone } from "@/shared/hooks/use-hub-time-zone";
import { PolicyControl } from "@/shared/components/updates/policy-control";
import { useUpdateAction } from "@/shared/components/updates/use-update-action";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { COMMAND_DIALOG_CONTENT } from "@/shared/components/dialog-layout";
import { useImageError } from "@/features/images/use-image-error";
import { RecreateRun } from "./RecreateRun";

export function ContainerUpdateDialog({ container: c, restore, close }: { container: ContainerInfo; restore: boolean; close: () => void }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), update = useTranslations("common.updates"), errors = useImageError();
  const cap = useCapability(restore ? "restoreContainerImage" : "installContainerUpdate", { kind: "container", id: c.id });
  const previous = useQuery({ queryKey: ["container", c.id, "previous-image"], queryFn: async () => (await adapter.getContainerPreviousImage!(c.id)) ?? null, enabled: restore && !!adapter.getContainerPreviousImage, staleTime: 0 });
  const action = useUpdateAction(c.id), [run, setRun] = useState<string>();
  useEffect(() => identityLifetime(adapter).listen(close), [adapter, close]);
  return <Dialog open onOpenChange={(open) => { if (!open) close(); }}><DialogContent className={COMMAND_DIALOG_CONTENT}><DialogHeader><DialogTitle>{restore ? t("previousImage") : update("actionUpdateContainer")}</DialogTitle><DialogDescription>{restore ? t("previousImageHelp") : c.name}</DialogDescription></DialogHeader>
    {restore && previous.data && <p className="break-all">{previous.data.reference} · {formatDateTime(previous.data.createdAt)}</p>}
    {restore && previous.isError && <p role="alert">{errors(previous.error)}</p>}
    {action.error && <p role="alert">{action.error}</p>}
    {run ? <RecreateRun hostId={c.hostId} runId={run} onNavigated={close} /> : <Button disabled={!cap.available || action.busy || (restore && (!previous.data || previous.isFetching || previous.isError)) || !!c.inventory?.stale} title={cap.reason} onClick={() => void action.run(() => restore ? adapter.restoreContainerImage!(c.id) : adapter.installContainerUpdate(c.id), setRun)}>{restore ? t("previousImage") : update("actionUpdateContainer")}</Button>}
  </DialogContent></Dialog>;
}
function StandaloneUpdates({ c, select }: { c: ContainerInfo; select: (restore: boolean) => void }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), update = useTranslations("common.updates"), errors = useImageError(), timeZone = useHubTimeZone();
  const cap = useCapability("installContainerUpdate", { kind: "container", id: c.id });
  const policyId = inventoryId(c.hostId, c.name);
  const read = useCapability("getAutoUpdatePolicy", { kind: "container", id: policyId });
  const policy = useQuery({ queryKey: ["container", policyId, "update-policy"], queryFn: () => adapter.getContainerUpdatePolicy!(policyId), enabled: read.available && !!adapter.getContainerUpdatePolicy });
  const previous = useQuery({ queryKey: ["container", c.id, "previous-image"], queryFn: async () => (await adapter.getContainerPreviousImage!(c.id)) ?? null, enabled: cap.available && !!adapter.getContainerPreviousImage });
  return <section className="flex flex-col gap-3 rounded-xl border p-4">
    {hasUpdate(c.update) && <Button disabled={!cap.available || !!c.inventory?.stale} title={cap.reason} onClick={() => select(false)}>{update("actionUpdateContainer")}</Button>}
    {previous.data && <div><p className="break-all text-xs">{previous.data.reference} · {formatDateTime(previous.data.createdAt)}</p><Button variant="outline" disabled={!!c.inventory?.stale} onClick={() => select(true)}>{t("previousImage")}</Button></div>}
    {previous.isError && <p role="alert">{errors(previous.error)}</p>}
    {policy.data && <PolicyControl policy={policy.data} timeZone={timeZone} saved={() => { void policy.refetch(); }} />}
    {policy.isError && <p role="alert">{errors(policy.error)}</p>}
  </section>;
}
export function ContainerUpdates({ c, select }: { c: ContainerInfo; select: (restore: boolean) => void }) {
  const adapter = useAdapter();
  if (adapter.kind !== "http" || c.origin !== "standalone" || isSystemWorkload(c) || c.redeployDisabled) return null;
  return <StandaloneUpdates key={`${c.id}:${c.name}`} c={c} select={select} />;
}
