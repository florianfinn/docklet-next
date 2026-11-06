// Source: docklet-next/issues/348
// Revision: b5068aff3da7eac11f208d1f4d8c9068d4a8c073
// Retrieved: 2026-11-06
// License: Apache-2.0
// Deviations: Original one-time import consumer with identity and draft ownership.

import { useCallback, useEffect, useLayoutEffect, useRef, useState } from "react";
import { useTranslations } from "use-intl";
import type { ImportRequest, ImportResult } from "@/data/adapter";
import { AdapterOutcomeError } from "@/data/capability";
import { identityLifetime } from "@/data/identity-lifetime";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import { IMPORT_FOUND_KEYS, IMPORT_MISSING_KEYS, importProblemMessage } from "@/data/wire-catalogue-import";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Field, FieldDescription, FieldLabel } from "@/shared/ui/field";
import { Input } from "@/shared/ui/input";
import { Tabs, TabsList, TabsTrigger } from "@/shared/ui/tabs";

interface Props { open: boolean; draftKey?: string; onOpenChange: (open: boolean) => void; onApply: (result: ImportResult) => void }
export function ImportSourceDialog({ open, draftKey = "new", onOpenChange, onApply }: Props) {
  const t = useTranslations("editor.source"), all = useTranslations(), tc = useTranslations("common.actions");
  const adapter = useAdapter(), capability = useCapability("importSource");
  const [kind, setKind] = useState<ImportRequest["kind"]>("github"), [ref, setRef] = useState("");
  const [pending, setPending] = useState(false), [result, setResult] = useState<ImportResult>(), [error, setError] = useState<string>();
  const generation = useRef(0);
  const closeRef = useRef(onOpenChange);
  useLayoutEffect(() => { closeRef.current = onOpenChange; });
  const clear = useCallback(() => { ++generation.current; setPending(false); setResult(undefined); setError(undefined); }, []);
  const [owner, setOwner] = useState({ open, draftKey, adapter });
  if (owner.open !== open || owner.draftKey !== draftKey || owner.adapter !== adapter) {
    setOwner({ open, draftKey, adapter }); setPending(false); setResult(undefined); setError(undefined);
  }
  // Layout cleanup fences a promise before it can publish into a different workspace.
  useLayoutEffect(() => { const fence = generation; ++fence.current; return () => { ++fence.current; }; }, [open, draftKey, adapter]);
  useEffect(() => identityLifetime(adapter).listen(() => { clear(); closeRef.current(false); }), [adapter, clear]);
  const fetchSource = async () => {
    clear();
    if (!ref.trim()) { setError(t("refMissing")); return; }
    if (!capability.available) return;
    const owner = generation.current;
    setPending(true);
    try { const value = await adapter.importSource({ kind, ref }); if (generation.current === owner) setResult(value); }
    catch (cause) { if (generation.current === owner) setError(importProblemMessage(cause instanceof AdapterOutcomeError && cause.outcome.kind === "error" ? cause.outcome.code : undefined, all)); }
    finally { if (generation.current === owner) setPending(false); }
  };
  return <Dialog open={open} onOpenChange={(next) => { clear(); onOpenChange(next); }}><DialogContent className="sm:max-w-xl"><DialogHeader><DialogTitle>{t("title")}</DialogTitle><DialogDescription>{t("description")}</DialogDescription></DialogHeader>
    <Tabs value={kind} onValueChange={(value) => { clear(); setRef(""); setKind(value as ImportRequest["kind"]); }}><TabsList><TabsTrigger value="github">{t("github")}</TabsTrigger><TabsTrigger value="dockerhub">{t("dockerhub")}</TabsTrigger></TabsList></Tabs>
    <form className="flex flex-col gap-3" onSubmit={(event) => { event.preventDefault(); void fetchSource(); }}><Field><FieldLabel htmlFor="import-ref">{t("ref")}</FieldLabel><Input id="import-ref" value={ref} onChange={(event) => { clear(); setRef(event.target.value); }} placeholder={t(kind === "github" ? "githubPlaceholder" : "dockerhubPlaceholder")} />{kind === "dockerhub" && <FieldDescription>{t("registriesHint")}</FieldDescription>}</Field><Button type="submit" disabled={pending || !capability.available}>{t("fetch")}</Button></form>
    {!capability.available && <p role="status">{capability.reason}</p>}{error && <p role="alert">{error}</p>}
    {result && <div className="flex flex-col gap-3"><p className="font-medium">{result.title}</p><p>{t("found")}</p><ul>{result.found.map((code) => <li key={code}>{all(IMPORT_FOUND_KEYS[code])}</li>)}</ul><p>{t("missing")}</p><ul>{result.missing.map((code) => <li key={code}>{all(IMPORT_MISSING_KEYS[code])}</li>)}</ul></div>}
    <DialogFooter><Button variant="ghost" onClick={() => { clear(); onOpenChange(false); }}>{tc("cancel")}</Button><Button disabled={pending || !result} onClick={() => { if (!result || pending) return; onApply(result); clear(); onOpenChange(false); }}>{t("apply")}</Button></DialogFooter>
  </DialogContent></Dialog>;
}
