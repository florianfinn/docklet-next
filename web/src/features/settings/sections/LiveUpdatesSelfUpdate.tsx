// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original capability-gated Compose self-update controls and confirmation; M21 R1 host-named controls and settled-state polling.

import { useEffect, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { SELF_UPDATE_STATE_KEYS, SELF_UPDATE_PHASE_KEYS, SELF_UPDATE_OUTCOME_KEYS } from "@/data/wire-catalogue-self-update";
import { canSelfUpdate, selfUpdateCodeKey, selfUpdateRef, selfUpdateState } from "@/domain/self-update";
import { openActivity } from "@/features/activity/activity-store";
import { useImageError } from "@/features/images/use-image-error";
import { useUpdateAction } from "@/shared/components/updates/use-update-action";
import { CodeBlock } from "@/shared/components/code-block";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/shared/ui/dialog";

export function LiveUpdatesSelfUpdate({ hostId, name }: { hostId: string; name?: string }) {
  const adapter = useAdapter(), all = useTranslations(), t = useTranslations("settings.updates.selfUpdate"), errorText = useImageError();
  const cap = useCapability("selfUpdate", selfUpdateRef(hostId));
  const query = useQuery({ queryKey: ["settings", "self-update", hostId], queryFn: () => adapter.getSelfUpdate(hostId), enabled: cap.available,
    // A live check reaches the agent; repeat it only while an operation is unsettled.
    refetchInterval: (q) => q.state.data?.operation && q.state.data.operation.phase !== "settled" ? 10_000 : false });
  const [confirm, setConfirm] = useState<string>(), [started, setStarted] = useState<string>();
  const action = useUpdateAction(hostId);
  useEffect(() => identityLifetime(adapter).listen(() => { setConfirm(undefined); setStarted(undefined); }), [adapter]);
  const state = selfUpdateState(query.data), op = query.data?.operation;
  const running = !!op && op.phase !== "settled";
  const enabled = cap.available && !query.isError && canSelfUpdate(state) && !action.busy && !running;
  const codeKey = selfUpdateCodeKey(state.code), opCodeKey = selfUpdateCodeKey(op?.code);
  const runId = started ?? op?.runId;
  const title = hostId === "0" ? t("hub") : name ? t("agentOn", { name }) : t("agent");
  return <section className="flex flex-col gap-3" aria-label={title}>
    <h4 className="font-semibold">{title}</h4>
    {!cap.available ? <p role="status">{cap.reason}</p> : <>
      {query.data && <p>{t("currentVersion", { version: query.data.version })}</p>}
      <p role="status">{all(SELF_UPDATE_STATE_KEYS[query.isError ? "unavailable" : state.state])}</p>
      {codeKey && <p>{all(codeKey)}</p>}
      {query.isError && <p role="alert">{errorText(query.error)}</p>}
      {state.candidate && <p>{t("targetVersion", { version: state.candidate.version })}</p>}
      {op && <p>{all(SELF_UPDATE_PHASE_KEYS[op.phase])}{op.outcome && <> · {all(SELF_UPDATE_OUTCOME_KEYS[op.outcome])}</>}{opCodeKey && <> · {all(opCodeKey)}</>}</p>}
      <div className="flex flex-wrap gap-2">
        <Button type="button" variant="outline" disabled={!enabled} onClick={() => setConfirm(state.candidate!.version)}>{title}</Button>
        <Button type="button" variant="outline" disabled={query.isFetching} aria-label={name ? t("checkFor", { name }) : undefined} onClick={() => { void query.refetch(); }}>{t("check")}</Button>
        {runId && <Button type="button" variant="outline" onClick={() => openActivity(runId)}>{t("viewRun")}</Button>}
      </div>
      {state.manual.map((command, index) => <CodeBlock key={`${index}:${command}`} code={command} label={t("manual", { number: index + 1 })} />)}
    </>}
    {action.error && <p role="alert">{action.error}</p>}
    <Dialog open={!!confirm && cap.available} onOpenChange={(open) => { if (!open) setConfirm(undefined); }}>
      <DialogContent>
        <DialogHeader><DialogTitle>{title}</DialogTitle><DialogDescription>{t(hostId === "0" ? "confirmHub" : "confirmAgent", { version: confirm ?? "" })}</DialogDescription></DialogHeader>
        <p>{t("noRollback")}</p>
        {hostId === "0" && <p>{t("backupAdvice")} <Link to="/hub-backups" className="text-primary-text underline">{t("backupLink")}</Link></p>}
        {!action.busy && (!enabled || confirm !== state.candidate?.version) && <p role="status">{t("changed")}</p>}
        {action.error && <p role="alert">{action.error}</p>}
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => setConfirm(undefined)}>{t("cancel")}</Button>
          <Button type="button" disabled={!enabled || !confirm || confirm !== state.candidate?.version} onClick={() => {
            if (!enabled || !confirm || confirm !== state.candidate?.version) return;
            void action.run(() => adapter.selfUpdate(hostId, { targetVersion: confirm }), (id) => { setStarted(id); setConfirm(undefined); void query.refetch(); });
          }}>{t("confirm")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>;
}
