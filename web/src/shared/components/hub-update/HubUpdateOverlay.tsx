// Source: docklet-next/issues/529
// Revision: 94ff3b632c90da82a96c28db8eefc0184a6adfd7
// Retrieved: 2026-11-19
// License: Apache-2.0
// Deviations: Original statically loaded, modal hub update view and bounded visual estimate. M37 #580: wordmark loader while running.

import { useEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import type { HubUpdateSession } from "@/data/http/hub-update-session";
import { useAuthMode } from "@/data/queries";
import { useAdapter } from "@/data/use-adapter";
import { selfUpdateCodeKey } from "@/domain/self-update";
import { openActivity } from "@/features/activity/activity-store";
import { BrandLoader } from "@/shared/brand/BrandLoader";
import { BrandLogo } from "@/shared/brand/BrandLogo";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogTitle } from "@/shared/ui/dialog";
import { useHubUpdate } from "./use-hub-update";

import { UPDATE_ESTIMATE, UPDATE_CLOSE_AFTER, updateProgress } from "./progress";

const reloadDocument = () => window.location.reload();
function Done({ session, reload }: { session: HubUpdateSession; reload: () => void }) {
  const t = useTranslations("common.hubUpdate"), [seconds, setSeconds] = useState(10), reloaded = useRef(false);
  const open = () => {
    const snapshot = session.get();
    if (reloaded.current || snapshot.state.phase !== "done" || snapshot.dismissed) return;
    reloaded.current = true; reload();
  };
  useEffect(() => {
    const until = Date.now() + 10_000;
    const timer = setInterval(() => { const left = Math.max(0, Math.ceil((until - Date.now()) / 1000)); setSeconds(left); if (!left) open(); }, 250);
    return () => clearInterval(timer);
    // The mounted terminal view owns one countdown; clearing its session also invalidates open().
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [session]);
  return <><Button onClick={open}>{t("open")}</Button><p role="status">{t("countdown", { seconds })}</p></>;
}
export function HubUpdateOverlay({ reload = reloadDocument }: { reload?: () => void }) {
  const adapter = useAdapter(), session = adapter.hubUpdateSession;
  const { state, dismissed } = useHubUpdate();
  const t = useTranslations("common.hubUpdate"), all = useTranslations();
  const mode = useAuthMode();
  const [now, setNow] = useState(Date.now);
  const previousFocus = useRef<HTMLElement | null>(null);
  useEffect(() => {
    if (state.phase !== "running" && state.phase !== "starting") return;
    const timer = setInterval(() => setNow(Date.now()), 1000);
    return () => clearInterval(timer);
  }, [state.phase]);
  if (!session || state.phase === "idle" || dismissed) return null;
  const progressing = state.phase === "starting" || state.phase === "running";
  const elapsed = progressing ? Math.max(0, now - state.startedAt) : 0;
  const canClose = state.phase === "failed" || state.phase === "unknown" || state.phase === "running" && elapsed >= UPDATE_CLOSE_AFTER;
  const runId = "runId" in state ? state.runId : undefined;
  const showRun = !!runId && "authenticated" in state && state.authenticated && (!progressing || elapsed >= UPDATE_CLOSE_AFTER);
  const code = state.phase === "failed" ? selfUpdateCodeKey(state.code) : undefined;
  const status = state.phase === "starting" ? "preparing" : state.phase === "running" ? !state.online ? "offline" : state.targetSeen ? "checking" : "waiting" : undefined;
  return <Dialog open modal>
    <DialogContent aria-modal="true" showCloseButton={false} data-hub-update-overlay="" className="inset-0 top-0 left-0 z-[120] h-svh w-screen max-w-none grid-cols-[minmax(0,1fr)] translate-x-0 translate-y-0 place-content-center justify-items-center rounded-none border-0 bg-background p-6 text-center font-sans sm:max-w-none" style={{ fontFamily: "system-ui, sans-serif" }} onOpenAutoFocus={() => { previousFocus.current = document.activeElement instanceof HTMLElement ? document.activeElement : null; }} onCloseAutoFocus={event => {
      event.preventDefault();
      const target = previousFocus.current?.isConnected && previousFocus.current !== document.body ? previousFocus.current : document.querySelector<HTMLElement>("#hub-update button:not(:disabled)") ?? document.querySelector<HTMLElement>("#hub-update h4");
      target?.focus();
    }} onEscapeKeyDown={event => event.preventDefault()} onPointerDownOutside={event => event.preventDefault()} onInteractOutside={event => event.preventDefault()}>
      {progressing ? <BrandLoader size={48} /> : <BrandLogo size={48} motion={false} />}
      <DialogTitle className="max-w-xl text-2xl">{state.phase === "starting" ? t("preparing") : t(state.phase === "running" ? "running" : state.phase, { version: state.target })}</DialogTitle>
      <DialogDescription className="max-w-xl">{state.phase === "signin" ? t(mode.data === "proxy" ? "signinProxy" : "signinHelp", { version: state.versionSeen ?? t("versionUnknown") }) : state.phase === "unknown" ? t("unknownHelp") : progressing ? t("estimate") : state.phase === "done" ? t("doneHelp") : t("failedHelp")}</DialogDescription>
      {progressing && <>
        <div role="progressbar" aria-label={t("progress")} aria-valuemin={0} aria-valuemax={100} aria-valuenow={updateProgress(elapsed)} className="h-2 w-full max-w-md overflow-hidden rounded-full bg-muted"><div className="h-full rounded-full bg-primary transition-[width] motion-reduce:transition-none" style={{ width: `${updateProgress(elapsed)}%` }} /></div>
        <p role="status">{status && t(status)} · {t("elapsed", { time: `${Math.floor(elapsed / 60_000)}:${String(Math.floor(elapsed / 1000) % 60).padStart(2, "0")}` })}</p>
        {elapsed >= UPDATE_ESTIMATE && <p>{t("longer")}</p>}
      </>}
      {code && <p role="alert">{all(code)}</p>}
      {state.phase === "failed" && state.code && !code && <p role="alert">{t("errorCode", { code: state.code })}</p>}
      {state.phase === "unknown" && state.versionSeen && <p>{t("versionSeen", { version: state.versionSeen })}</p>}
      {state.phase === "done" && <Done session={session} reload={reload} />}
      {state.phase === "signin" && <Button onClick={reload}>{t(mode.data === "proxy" ? "reload" : "signinAction")}</Button>}
      {showRun && <Button variant="outline" onClick={() => { session.dismiss(); openActivity(runId); }}>{t("viewRun")}</Button>}
      {runId && canClose && <p className="max-w-xl break-all text-xs text-muted-foreground">{t("runId", { id: runId })}</p>}
      {canClose && <Button variant="outline" onClick={() => session.dismiss()}>{t("close")}</Button>}
    </DialogContent>
  </Dialog>;
}
