// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original confirmed deletion and uncertain-outcome reconciliation for issue #231.

import { useEffect, useRef, useState } from "react";
import { useLocation } from "react-router";
import { useTranslations } from "use-intl";
import type { UpdatePoint } from "@/domain/stack-updates";
import type { ResourceRef } from "@/data/capability";
import { AdapterOutcomeError } from "@/data/capability";
import { useImageError } from "@/features/images/use-image-error";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription } from "@/shared/ui/dialog";
import { RestorePointState } from "./restore-point-state";
import { UpdateUnavailable } from "./live-state";
import { useUpdateAction } from "./use-update-action";

export function DeletePointDialog({ scope, point, latest, readError, refresh, close }: {
  scope: ResourceRef; point: UpdatePoint; latest?: UpdatePoint; readError?: unknown; refresh: () => Promise<UpdatePoint[]>; close: () => void;
}) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), location = useLocation(), errorText = useImageError();
  const owner = `${scope.kind}:${scope.id}:${point.registrationId}:${point.targetIdentity}:${point.id}`;
  const action = useUpdateAction(owner), reading = useUpdateAction(owner);
  const [current, setCurrent] = useState(point), [uncertain, setUncertain] = useState(false), [reconciled, setReconciled] = useState(false);
  const [showRefusal, setShowRefusal] = useState(true);
  const [deleted, setDeleted] = useState(false), [denied, setDenied] = useState(false);
  const sending = useRef(false), openedAt = useRef(location.key);
  useEffect(() => identityLifetime(adapter).listen(close), [adapter, close]);
  useEffect(() => { if (openedAt.current !== location.key) close(); }, [location.key, close]);
  const host = scope.kind === "host";
  const read = useCapability(host ? "listHostRestorePoints" : "listRestorePoints", scope);
  const write = useCapability(host ? "deleteHostRestorePoint" : "deleteRestorePoint", scope);
  const displayed = latest ?? current;
  const sameOwner = displayed.targetIdentity === point.targetIdentity && displayed.registrationId === point.registrationId;
  useEffect(() => { if (!sameOwner) close(); }, [sameOwner, close]);
  const held = displayed.holdState !== "none";

  function reconcile(unknown: boolean) {
    void reading.run(refresh, (rows) => {
      const fresh = rows.find((p) => p.id === point.id);
      if (!fresh) { if (unknown) setDeleted(true); else close(); return; }
      setCurrent(fresh); setReconciled(true);
      if (unknown && !denied) action.reset();
    });
  }
  function remove() {
    if (readError !== undefined || !sameOwner || sending.current || action.busy || reading.busy || deleted || held || denied || !read.available || !write.available || (uncertain && !reconciled)) return;
    sending.current = true; setShowRefusal(true);
    void action.run(async () => {
      try {
        if (host) await adapter.deleteHostRestorePoint!(scope.id, point.id);
        else await adapter.deleteRestorePoint!(scope.id, point.id);
      } finally { sending.current = false; }
    }, () => { setDeleted(true); void reading.run(refresh); }, (error) => {
      if (!(error instanceof AdapterOutcomeError) || error.outcome.kind !== "error") return;
      const { code, status } = error.outcome;
      if (status === 401 || status === 403) setDenied(true);
      if (code === "runtime.admission-unknown" || code === "runtime.outcome-unknown") {
        setShowRefusal(false); setUncertain(true); setReconciled(false); reconcile(true);
      } else if (code === "restore.not-found" && uncertain) setDeleted(true);
      else if (code === "restore.in-use") reconcile(false);
    });
  }
  return <Dialog open onOpenChange={(v) => { if (!v) close(); }}><DialogContent><DialogHeader>
    <DialogTitle>{t("deletePoint")}</DialogTitle><DialogDescription>{t("deleteHelp")}</DialogDescription>
  </DialogHeader>
    <p>{t("point", { at: point.createdAt })}</p><p className="break-all">{point.targetId} · {point.id}</p><RestorePointState point={displayed} />
    {deleted ? <p role="status">{t(uncertain ? "alreadyDeleted" : "deleted")}</p> : <>
      {(!read.available || !write.available) && <UpdateUnavailable reason={read.reason ?? write.reason} />}
      {uncertain && <p role="status">{t("deleteUnknown")}</p>}
      {uncertain && reconciled && <p>{t("deleteStillPresent")}</p>}
      {showRefusal && action.error && <p role="alert">{action.error}</p>}
      {(readError !== undefined || reading.error) && <p role="alert">{readError !== undefined ? errorText(readError) : reading.error}</p>}
      {(uncertain || readError !== undefined) && <Button variant="outline" disabled={!read.available || reading.busy} onClick={() => reconcile(uncertain)}>{t("refreshPoints")}</Button>}
      <Button variant="destructive" disabled={readError !== undefined || !sameOwner || !read.available || !write.available || action.busy || reading.busy || held || denied || (uncertain && !reconciled)} onClick={remove}>{t("deletePoint")}</Button>
    </>}
    <Button variant="outline" onClick={close}>{t("close")}</Button>
  </DialogContent></Dialog>;
}
