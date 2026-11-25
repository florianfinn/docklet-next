// Source: docklet-next/issues/439
// Revision: 48f47c93878ecf1c27491d0214db80d5b38b547d
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original capability-gated Compose self-update controls and confirmation; M21 R1 host-named controls and settled-state polling; recheck refreshes the release list and shows its time. M40 #615 #616: shared identity-owned check read, version badges, collapsed manual commands and agent anchors.

import { useEffect, useRef, useState } from "react";
import { Link, useLocation } from "react-router";
import { useFormatter, useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { SELF_UPDATE_STATE_KEYS, SELF_UPDATE_PHASE_KEYS, SELF_UPDATE_OUTCOME_KEYS } from "@/data/wire-catalogue-self-update";
import { canSelfUpdate, hostUpdateAnchor, isHostUpdateHash, selfUpdateCodeKey, selfUpdateRef, selfUpdateState, versionBadgeModel } from "@/domain/self-update";
import { openActivity } from "@/features/activity/activity-store";
import { useImageError } from "@/features/images/use-image-error";
import { useUpdateAction } from "@/shared/components/updates/use-update-action";
import { useSelfUpdateCheck } from "@/shared/components/updates/use-self-update-check";
import type { Host } from "@/domain/model";
import { CodeBlock } from "@/shared/components/code-block";
import { VersionBadges } from "@/shared/components/updates/version-badges";
import { Collapsible, CollapsibleContent, CollapsibleTrigger } from "@/shared/ui/collapsible";
import { ChevronRight } from "lucide-react";
import { keepInView } from "@/shared/lib/keep-in-view";
import { Button } from "@/shared/ui/button";
import { Spinner } from "@/shared/ui/spinner";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogDescription, DialogFooter } from "@/shared/ui/dialog";

export function LiveUpdatesSelfUpdate({ hostId, name, host }: { hostId: string; name?: string; host?: Pick<Host, "identity" | "agentVersion"> }) {
  const adapter = useAdapter(), all = useTranslations(), t = useTranslations("settings.updates.selfUpdate"), errorText = useImageError();
  const location = useLocation(), heading = useRef<HTMLHeadingElement>(null), button = useRef<HTMLButtonElement>(null), focusedFor = useRef<string>(undefined), release = useRef<() => void>(undefined);
  const cap = useCapability("selfUpdate", selfUpdateRef(hostId));
  const format = useFormatter(), { query, recheck: fetchFresh } = useSelfUpdateCheck(hostId, { host, enabled: cap.available });
  const [confirm, setConfirm] = useState<string>(), [started, setStarted] = useState<string>();
  const [rechecking, setRechecking] = useState(false), [checkedAt, setCheckedAt] = useState<Date>();
  const [toggled, setToggled] = useState<boolean>(), [toggleClass, setToggleClass] = useState<boolean>();
  const recheck = async () => {
    setRechecking(true);
    try {
      await fetchFresh();
      setCheckedAt(new Date());
    } catch { setCheckedAt(undefined); } finally { setRechecking(false); }
  };
  const action = useUpdateAction(hostId);
  useEffect(() => identityLifetime(adapter).listen(() => { setConfirm(undefined); setStarted(undefined); setCheckedAt(undefined); }), [adapter]);
  const state = selfUpdateState(query.data), op = query.data?.operation;
  const running = !!op && op.phase !== "settled";
  const enabled = cap.available && !query.isError && canSelfUpdate(state) && !action.busy && !running;
  useEffect(() => {
    if (!isHostUpdateHash(hostId, location.hash) || cap.pending || cap.available && query.isPending || focusedFor.current === location.key) return;
    // Focus once per navigation to the anchor, not again on later state changes.
    focusedFor.current = location.key;
    const target = button.current && !button.current.disabled ? button.current : heading.current;
    target?.scrollIntoView?.({ block: "center" }); target?.focus();
    // Policies and other hosts above may still load and push this panel down.
    release.current?.(); release.current = target ? keepInView(target) : undefined;
  }, [hostId, location.hash, location.key, query.isPending, enabled, cap.pending, cap.available]);
  useEffect(() => () => release.current?.(), []);
  const codeKey = selfUpdateCodeKey(state.code), opCodeKey = selfUpdateCodeKey(op?.code);
  const runId = started ?? op?.runId;
  const title = hostId === "0" ? t("hub") : name ? t("agentOn", { name }) : t("agent");
  const badges = versionBadgeModel({ reported: host?.agentVersion, check: query.data, allowed: cap.available, status: !cap.available ? "off" : query.isError ? "error" : query.isSuccess ? "success" : "pending" });
  // The badges carry current and available; the state line remains for every other state and the check time.
  const shownState = query.isError ? "unavailable" : state.state, stateLine = shownState !== "current" && shownState !== "available";
  // Commands open by default only when the automatic update is not possible; a failed read carries none.
  const autoOpen = !query.isError && (state.state === "not-eligible" || state.state === "unavailable") && state.code !== "host.busy" && !running;
  // A user toggle lasts until the default-open class changes; returning to the old class does not restore it.
  if (toggleClass !== autoOpen) { setToggleClass(autoOpen); setToggled(undefined); }
  const commandsOpen = toggleClass === autoOpen ? toggled ?? autoOpen : autoOpen;
  return <section id={hostUpdateAnchor(hostId)} className="flex scroll-mt-24 flex-col gap-3" aria-label={title}>
    <div className="flex flex-wrap items-center gap-x-3 gap-y-1.5">
      <h4 ref={heading} tabIndex={-1} className="font-semibold">{name ?? title}</h4>
      {cap.available && <VersionBadges model={badges} variant="panel" />}
    </div>
    {!cap.available ? <p role="status">{cap.reason}</p> : <>
      <p role="status" className={stateLine ? undefined : "text-sm text-muted-foreground empty:hidden"}>{stateLine && all(SELF_UPDATE_STATE_KEYS[shownState])}{checkedAt && !rechecking && <span className="text-muted-foreground">{stateLine && " · "}{t("checkedAt", { time: format.dateTime(checkedAt, { timeStyle: "short" }) })}</span>}</p>
      {codeKey && <p>{all(codeKey)}</p>}
      {query.isError && <p role="alert">{errorText(query.error)}</p>}
      {op && <p>{all(SELF_UPDATE_PHASE_KEYS[op.phase])}{op.outcome && <> · {all(SELF_UPDATE_OUTCOME_KEYS[op.outcome])}</>}{opCodeKey && <> · {all(opCodeKey)}</>}</p>}
      <div className="flex flex-wrap gap-2">
        <Button ref={button} type="button" variant={enabled ? "default" : "outline"} disabled={!enabled} aria-label={title} onClick={() => setConfirm(state.candidate!.version)}>{t(hostId === "0" ? "hub" : "agent")}</Button>
        <Button type="button" variant="outline" disabled={query.isFetching || rechecking} aria-label={name ? t("checkFor", { name }) : undefined} onClick={() => { void recheck(); }}>{rechecking && <Spinner data-icon="inline-start" />}{t("check")}</Button>
        {runId && <Button type="button" variant="ghost" onClick={() => openActivity(runId)}>{t("viewRun")}</Button>}
      </div>
      {state.manual.length > 0 && <Collapsible open={commandsOpen} onOpenChange={setToggled} className="flex flex-col gap-2">
        <CollapsibleTrigger asChild><Button type="button" variant="ghost" size="sm" className="w-fit gap-1 px-1 text-muted-foreground"><ChevronRight aria-hidden className={commandsOpen ? "rotate-90 transition-transform" : "transition-transform"} />{t("manualCommands", { count: state.manual.length })}</Button></CollapsibleTrigger>
        <CollapsibleContent className="flex flex-col gap-2">{state.manual.map((command, index) => <CodeBlock key={`${index}:${command}`} code={command} label={t("manual", { number: index + 1 })} />)}</CollapsibleContent>
      </Collapsible>}
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
            const targetVersion = confirm;
            if (hostId === "0") setConfirm(undefined);
            void action.run(() => adapter.selfUpdate(hostId, { targetVersion }), (id) => { setStarted(id); setConfirm(undefined); void query.refetch(); });
          }}>{t("confirm")}</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  </section>;
}
