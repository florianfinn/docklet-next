// Source: docklet-next/docs/design/updates-ui.md
// Revision: 5cb649fa7d72146ff0a078f5d6c7ea5c5b3e8454
// Retrieved: 2026-10-26
// License: Apache-2.0
// Deviations: Original live update UI for issue #231.

import { useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import { isCancelledError } from "@tanstack/react-query";
import { useAdapter } from "@/data/use-adapter";
import { isRunBusy } from "@/data/run-gate";
import { identityLifetime } from "@/data/identity-lifetime";
import { useImageError } from "@/features/images/use-image-error";
import { AdapterOutcomeError } from "@/data/capability";

export function useUpdateAction(ownerKey = "") {
  const adapter = useAdapter(), errorText = useImageError(), t = useTranslations("activity.runtime");
  const [error, setError] = useState<string>(), [busy, setBusy] = useState(false), [blocked, setBlocked] = useState(false);
  const [intentOwner, setIntentOwner] = useState(ownerKey);
  const sequence = useRef(0), mounted = useRef(true), currentOwner = useRef(ownerKey);
  useLayoutEffect(() => { if (currentOwner.current !== ownerKey) { ++sequence.current; currentOwner.current = ownerKey; } }, [ownerKey]);
  useEffect(() => {
    const supersede = () => { ++sequence.current; };
    mounted.current = true;
    const off = identityLifetime(adapter).listen(() => { supersede(); setError(undefined); setBusy(false); setBlocked(false); });
    return () => { mounted.current = false; supersede(); off(); };
  }, [adapter]);
  const reset = () => { ++sequence.current; setError(undefined); setBusy(false); setBlocked(false); };
  async function run<T>(action: () => Promise<T>, done?: (value: T) => void) {
    const ticket = ++sequence.current, life = identityLifetime(adapter), generation = life.version(), owner = currentOwner.current;
    const owns = () => currentOwner.current === owner && mounted.current && ticket === sequence.current && generation === life.version() && life.active();
    setIntentOwner(owner); setBusy(true); setError(undefined); setBlocked(false);
    try { const result = await action(); if (owns()) done?.(result); }
    catch (e) { if (owns() && !isCancelledError(e)) { if (e instanceof AdapterOutcomeError && e.outcome.kind === "error" && (e.outcome.status === 403 || e.outcome.status === 401 || (e.outcome.code === "runtime.admission-unknown" || e.outcome.code === "runtime.outcome-unknown"))) setBlocked(true); setError(isRunBusy(e) ? t("busy") : e instanceof AdapterOutcomeError && e.outcome.kind === "error" && e.outcome.code === "runtime.admission-unknown" ? t("ambiguous") : e instanceof AdapterOutcomeError && e.outcome.kind === "conflict" ? t(e.outcome.code === "run.cancelling" ? "cancellingConflict" : e.outcome.code === "run.request-mismatch" ? "mismatch" : "busy") : errorText(e)); } }
    finally { if (owns()) setBusy(false); }
  }
  return { error: intentOwner === ownerKey ? error : undefined, busy: intentOwner === ownerKey && (busy || blocked), run, reset };
}
