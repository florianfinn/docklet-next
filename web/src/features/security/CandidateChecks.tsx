// Source: docklet-next/issues/438
// Revision: cc93f43359147e8e128b2ac00bcd410c055eb423
// Retrieved: 2026-11-09
// License: Apache-2.0
// Deviations: Original candidate proof consumer and verified update integration. M21 R1-S12W B2 N1 N3 N5: stable refresh, single refusal text, newest runs.

import { useEffect, useRef, useState, type ReactNode } from "react";
import { useQuery } from "@tanstack/react-query";
import { openActivity } from "@/features/activity/activity-store";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { AdapterOutcomeError } from "@/data/capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { IMAGE_ERROR_KEYS } from "@/data/wire-catalogue-errors";
import { CANDIDATE_STATE_KEYS, CANDIDATE_RUN_STATUS_KEYS } from "@/data/wire-catalogue-states";
import { scopedKey, wireKey } from "@/data/wire-catalogue";
import type { CandidateResult, CandidateRun } from "@/data/vulnerability-candidates";
import type { AdvisoryOccurrence } from "@/data/vulnerability-advisory";
import { Button } from "@/shared/ui/button";
import { Alert, AlertDescription } from "@/shared/ui/alert";
import { AlertDialog, AlertDialogAction, AlertDialogCancel, AlertDialogContent, AlertDialogDescription, AlertDialogFooter, AlertDialogHeader, AlertDialogTitle, AlertDialogTrigger } from "@/shared/ui/alert-dialog";
import { CandidateContext, useCandidateContext } from "./candidate-context";
import { currentProof } from "./candidate-proof";
import { useCandidates } from "./use-candidates";

export function CandidateState({ state }: { state: CandidateResult["state"] }) {
  const t = useTranslations("security.candidates.states");
  return <span>{t(scopedKey(CANDIDATE_STATE_KEYS[state], "security.candidates.states."))}</span>;
}
export function CandidateRunStatus({ status }: { status: CandidateRun["status"] }) {
  const t = useTranslations("security.historyStatus");
  return <span>{t(scopedKey(CANDIDATE_RUN_STATUS_KEYS[status], "security.historyStatus."))}</span>;
}
function CandidateError({ code }: { code?: string }) {
  const t = useTranslations("images.errors"), security = useTranslations("security.candidates");
  const key = wireKey(IMAGE_ERROR_KEYS, code);
  return <span>{key ? t(scopedKey(key, "images.errors.")) : security("failed")}</span>;
}
function codeOf(error: unknown): string | undefined {
  return error instanceof AdapterOutcomeError ? ("code" in error.outcome ? error.outcome.code : error.outcome.reasonCode) : error && typeof error === "object" && "code" in error ? String(error.code) : undefined;
}
export function CandidateChecks({ host, advisory, children }: { host: string; advisory: string; children: ReactNode }) {
  const t = useTranslations("security.candidates"), adapter = useAdapter();
  const capability = useCapability("scan-vulnerability-candidates", { kind: "host", id: host });
  const descriptor = useQuery({ queryKey: ["capability", "scan-vulnerability-candidates", "host", host], queryFn: () => adapter.getCapability("scan-vulnerability-candidates", { kind: "host", id: host }) });
  const read = useCandidates(host, advisory);
  const [busy, setBusy] = useState(false), [error, setError] = useState<string>();
  const alive = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const check = async () => {
    const lifetime = identityLifetime(adapter), identity = lifetime.version();
    setBusy(true); setError(undefined);
    try { const id = await adapter.scanVulnerabilityCandidates(host, { advisory }); if (alive.current && lifetime.active() && lifetime.version() === identity) read.follow(id); }
    catch (error) { if (alive.current && lifetime.version() === identity) setError(codeOf(error) ?? "http.error"); }
    finally { if (alive.current && lifetime.version() === identity) setBusy(false); }
  };
  // A refresh keeps the shown verdict, offer and an open confirmation.
  return <CandidateContext value={{ data: read.data, unavailable: !!read.error || !read.active, reload: read.reload }}>
    {advisory && read.active && codeOf(read.error) !== "feature-disabled" && capability.available && <Button size="sm" variant="outline" disabled={busy} onClick={() => void check()}>{t("check")}</Button>}
    {advisory && !capability.pending && !capability.available && <p className="text-sm text-muted-foreground">{descriptor.data?.disabledReasonCode === "feature-disabled" ? <CandidateError code="feature-disabled" /> : capability.reason}</p>}
    {!!(error || (read.error && capability.available)) && <Alert><AlertDescription><CandidateError code={error ?? codeOf(read.error)} /></AlertDescription></Alert>}
    {read.activityId && <Button variant="link" size="sm" onClick={() => openActivity(read.activityId)}>{t("activity")}</Button>}
    {read.data?.runs.slice(0, 3).map((run) => <p key={run.id} className="text-xs text-muted-foreground">
      {t(run.trigger)} · <CandidateRunStatus status={run.status} /> · {t("counts", run)} · <Button variant="link" size="sm" onClick={() => openActivity(run.activityId)}>{t("activity")}</Button>
    </p>)}
    {children}
  </CandidateContext>;
}

export function CandidateVerdict({ host, occurrence: o, container, update: observation }: { host: string; occurrence: AdvisoryOccurrence; container: AdvisoryOccurrence["containers"][number]; update?: AdvisoryOccurrence["updates"][number] }) {
  const context = useCandidateContext(), adapter = useAdapter(), t = useTranslations("security.candidates"), detail = useTranslations("security.detail");
  const candidate = context?.data?.results.find((r) => r.consumer.imageId === o.imageId && r.consumer.containerId === container.id && r.consumer.projectId === container.projectId && r.consumer.service === container.service);
  const verdict = candidate?.verdicts.find((v) => v.advisory === o.vulnerabilityId && v.package === o.pkgName && v.installed === o.installedVersion && v.class === (o.class ?? "") && v.type === (o.type ?? ""))?.verdict;
  const stack = !!container.projectId, target = `${host}~${container.projectId ?? container.id}`;
  const capability = useCapability(stack ? "installStackUpdates" : "installContainerUpdate", { kind: stack ? "stack" : "container", id: target });
  const baselineCapability = useCapability("getUpdateBaseline", { kind: "stack", id: target });
  const [dialog, setDialog] = useState<{ digest: string; revision?: string }>();
  const [busy, setBusy] = useState(false), [error, setError] = useState<string>(), [rejected, setRejected] = useState<string>();
  const [activity, setActivity] = useState<{ id: string; digest: string }>();
  const alive = useRef(false);
  useEffect(() => { alive.current = true; return () => { alive.current = false; }; }, []);
  const digest = candidate?.identity?.manifestDigest;
  const verified = !context?.unavailable && currentProof(candidate) && verdict === "closes" && rejected !== digest;
  const reload = context?.reload;
  useEffect(() => {
    if (!activity) return;
    const lifetime = identityLifetime(adapter), identity = lifetime.version();
    let live = true;
    const stop = adapter.observeActionDetail?.(host, activity.id, (state) => {
      if (!live || lifetime.version() !== identity || !lifetime.active()) return;
      if (state.status !== "ready" || ["running", "queued"].includes(state.run.status)) return;
      if (state.run.lifecycleCode === "update.candidate-changed") {
        setError("update.candidate-changed"); setRejected(activity.digest);
      }
      setActivity(undefined); reload?.();
    });
    return () => { live = false; stop?.(); };
  }, [adapter, host, activity, reload]);
  const prepare = async () => {
    if (!verified || !digest || !currentProof(candidate)) return;
    const lifetime = identityLifetime(adapter), identity = lifetime.version();
    setBusy(true); setError(undefined);
    try {
      const baseline = stack ? await adapter.getUpdateBaseline!(target) : undefined;
      if (alive.current && lifetime.active() && lifetime.version() === identity) {
        if (stack && !baseline?.available) setError(baseline?.code ?? "update.baseline-unavailable");
        else setDialog({ digest, revision: baseline?.savedRevision });
      }
    } catch (error) { if (alive.current && lifetime.version() === identity) setError(codeOf(error) ?? "http.error"); }
    finally { if (alive.current && lifetime.version() === identity) setBusy(false); }
  };
  const update = async () => {
    const proof = dialog; setDialog(undefined);
    if (!proof || !verified || proof.digest !== digest || !currentProof(candidate)) return;
    const lifetime = identityLifetime(adapter), identity = lifetime.version();
    setBusy(true);
    try {
      const id = stack ? await adapter.installStackUpdates(target, { service: container.service!, expectedRevision: proof.revision!, expectedManifest: proof.digest }) : await adapter.installContainerUpdate(target, { expectedManifest: proof.digest });
      if (alive.current && lifetime.active() && lifetime.version() === identity) setActivity({ id, digest: proof.digest });
    } catch (error) {
      if (!alive.current || lifetime.version() !== identity) return;
      const code = codeOf(error); setError(code ?? "http.error");
      if (code === "update.candidate-changed") { setRejected(proof.digest); context?.reload(); }
    } finally { if (alive.current && lifetime.version() === identity) setBusy(false); }
  };
  if (!context) return null;
  return <div className="flex flex-col items-start gap-1">
    {!verified && <p>{observation?.state === "available" ? detail("updateAvailable", { version: observation.version || detail("versionUnknown") }) : observation?.state === "none" ? detail("noUpdate") : detail("updateUnknown")}</p>}
    {verified ? <div title={digest}>{t("closes", { reference: candidate!.consumer.reference, digest: digest!.slice(0, 19) })}<details><summary>{t("digest")}</summary><code>{digest}</code></details></div>
      : candidate?.code === "candidate.unavailable" || candidate?.state === "failed" ? <p><CandidateError code={candidate.reason ?? candidate.code} /></p>
      : currentProof(candidate) && verdict === "does-not-close" ? <p>{t("doesNotClose", { reference: candidate!.consumer.reference })}</p>
      : <p><CandidateState state={candidate && ["queued", "running", "cancelled", "interrupted", "deferred"].includes(candidate.state) ? candidate.state : "unchecked"} /></p>}
    {verified && capability.available && (!stack || (container.service && baselineCapability.available)) && <AlertDialog open={!!dialog} onOpenChange={(open) => { if (!open) setDialog(undefined); }}>
      <AlertDialogTrigger asChild><Button size="sm" disabled={busy || !!activity} onClick={() => void prepare()}>{t("update")}</Button></AlertDialogTrigger>
      <AlertDialogContent><AlertDialogHeader><AlertDialogTitle>{t("update")}</AlertDialogTitle><AlertDialogDescription className="break-all">{t("confirm", { consumer: stack ? `${container.project ?? container.projectId} / ${container.service}` : container.name, digest: dialog?.digest ?? digest! })}</AlertDialogDescription></AlertDialogHeader><AlertDialogFooter><AlertDialogCancel>{t("cancel")}</AlertDialogCancel><AlertDialogAction disabled={busy || !verified || !dialog || dialog.digest !== digest} onClick={() => void update()}>{t("confirmAction")}</AlertDialogAction></AlertDialogFooter></AlertDialogContent>
    </AlertDialog>}
    {error && <p role="alert"><CandidateError code={error} /></p>}
    {activity && <Button variant="link" size="sm" onClick={() => openActivity(activity.id)}>{t("activity")}</Button>}
    {!!candidate?.introduces.length && <div><p>{t("introduces")}</p><ul>{candidate.introduces.map((item, i) => <li key={i}>{item.advisory} · {item.package} {item.installed} · {t(`severity.${item.severity.toLowerCase() as "high"}`)}</li>)}</ul></div>}
  </div>;
}
