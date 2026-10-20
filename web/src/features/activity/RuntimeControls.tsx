// Source: docklet-next/docs/design/runtime-control.md
// Revision: f0447218f504f481d9e941df04bb6a19716ed6d7203060744204c44c3cc7cda8
// Retrieved: 2026-10-20
// License: Apache-2.0
// Deviations: Original guarded command and notification consumer for issues #126 and #110.

import { useEffect, useRef, useState } from "react";
import { isCancelledError } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { ActionRun } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { Input } from "@/shared/ui/input";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { runtimePresentation, runtimeState } from "./runtime-presentation";
import { useRuntimeError } from "./use-runtime-error";

/** The acknowledgment records operator-controlled facts; the server rechecks authority. */
export function RuntimeControls({ run }: { run: ActionRun }) {
  const adapter = useAdapter();
  const t = useTranslations("activity.runtime");
  const reportError = useRuntimeError();
  const [pending, setPending] = useState(false);
  const [reason, setReason] = useState("");
  const [acknowledged, setAcknowledged] = useState(false);
  const epoch = useRef(0);
  const host = run.hostId, id = run.id, runtime = run.outcomeState !== undefined;
  useEffect(() => {
    const current = epoch;
    const version = ++current.current;
    if (runtime && host) void adapter.getRuntimeRun?.(host, id).catch(() => undefined);
    return () => { if (current.current === version) ++current.current; };
  }, [adapter, host, id, runtime]);
  if (!runtime || !host) return null;
  const state = runtimeState(run), evidence = runtimePresentation(run);
  const active = run.status === "running" || run.status === "queued";
  const execute = async (action: () => Promise<unknown> | undefined) => {
    if (pending) return;
    const version = epoch.current;
    setPending(true);
    try { await action(); }
    catch (error) { if (epoch.current === version && !isCancelledError(error)) reportError(error); }
    finally { if (epoch.current === version) setPending(false); }
  };
  return <section className="flex min-w-0 flex-col gap-2 rounded-lg border p-3 text-xs" aria-label={t("controls")}>
    {evidence?.permissionsUnavailable && <p>{t("permissionsUnavailable")}</p>}
    {state && <p role="status">{t(`${state}Body`)}</p>}
    {active && run.cancelRequested && state !== "cancelling" && <p>{t("cancellingBody")}</p>}
    {run.status === "cancelled" && <p>{t("cancelledBody")}</p>}
    {evidence?.releaseReason && <p className="break-words">{t("releaseReason", { reason: evidence.releaseReason })}</p>}
    {evidence?.lateResult && <p>{t("lateEvidence")}</p>}
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="outline" disabled={pending} onClick={() => void execute(() => adapter.getRuntimeRun?.(host, id))}>{t("refresh")}</Button>
      {active && evidence?.canCancel && <Button size="sm" variant="outline" disabled={pending || run.cancelRequested} onClick={() => void execute(() => adapter.cancelRuntimeRun?.(host, id))}>{t("cancel")}</Button>}
    </div>
    {state === "unknown" && active && evidence?.canRelease && <div className="flex flex-col gap-2">
      <Label htmlFor={`release-reason-${id}`}>{t("reason")}</Label>
      <Input id={`release-reason-${id}`} value={reason} maxLength={2048} onChange={(event) => setReason(event.target.value)} disabled={pending} />
      <Label className="items-start leading-relaxed"><Checkbox checked={acknowledged} onCheckedChange={(value) => setAcknowledged(value === true)} disabled={pending} />{t("acknowledge")}</Label>
      <Button size="sm" variant="outline" disabled={pending || !acknowledged || !reason.trim()} onClick={() => void execute(() => adapter.releaseRuntimeRun?.(host, id, { acknowledgeRisk: true, reason }))}>{t("release")}</Button>
    </div>}
  </section>;
}
