// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original receipt-driven recreate progress, catalogued step names and replacement navigation.

import { useQueryClient } from "@tanstack/react-query";
import { useEffect, useRef, useState, useSyncExternalStore } from "react";
import { Link, useNavigate } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import type { ActionDetailState } from "@/data/adapter";
import { identityLifetime } from "@/data/identity-lifetime";
import { renderRun } from "@/data/message-code";
import { queryKeys } from "@/data/query-keys";
import { useAdapter } from "@/data/use-adapter";
import type { ActionRun } from "@/domain/model";
import { inventoryId } from "@/data/http/inventory-mapping";
import { Button } from "@/shared/ui/button";
import { containerHref } from "./container-links";

export function RecreateResultLink({ run }: { run: ActionRun }) {
  const t = useTranslations("containers.recreate");
  if (run.status !== "succeeded" || !run.recreateResult?.newId || !run.hostId) return null;
  const id = run.hostId && run.containerId?.includes("~") ? inventoryId(run.hostId, run.recreateResult.newId) : run.recreateResult.newId;
  return <Button asChild variant="outline" size="sm"><Link to={containerHref(id)}>{t("openReplacement")}</Link></Button>;
}

// Backend run step names (recreate_journal.go) with catalogued EN/DE text.
const STEP_KEYS = { "Renaming container": "renaming", "Creating replacement": "creating", "Starting replacement": "starting", "Removing previous container": "removing", "Restoring previous container": "restoring" } as const;

export function RecreateRun({ hostId, runId, onNavigated }: { hostId: string; runId: string; onNavigated?: () => void }) {
  const adapter = useAdapter();
  const client = useQueryClient();
  const navigated = useRef(onNavigated);
  useEffect(() => { navigated.current = onNavigated; });
  const navigate = useNavigate();
  const lifetime = identityLifetime(adapter);
  const generation = useSyncExternalStore(lifetime.listen, lifetime.version, lifetime.version);
  const key = `${hostId}:${runId}:${generation}`;
  const locale = useLocale();
  const t = useTranslations("containers.recreate");
  const ta = useTranslations("activity");
  const stepText = (label: string) => { const key = STEP_KEYS[label as keyof typeof STEP_KEYS]; return key ? t(`steps.${key}`) : label; };
  const [view, setView] = useState<{ key: string; state: ActionDetailState }>();
  const state: ActionDetailState = view?.key === key ? view.state : { status: "loading" };
  useEffect(() => {
    if (!lifetime.active()) return;
    let current = true;
    const off = adapter.observeActionDetail?.(hostId, runId, (value) => { if (current && generation === lifetime.version()) setView({ key, state: value }); });
    return () => { current = false; off?.(); };
  }, [adapter, hostId, runId, generation, key, lifetime]);
  const coded = state.status === "ready" || state.status === "error" ? state.run : undefined;
  const run = coded ? renderRun(coded, locale) : undefined;
  const target = run?.status === "succeeded" && run.recreateResult?.newId ? (run.containerId?.includes("~") ? inventoryId(hostId, run.recreateResult.newId) : run.recreateResult.newId) : undefined;
  useEffect(() => {
    if (!target || !lifetime.active() || generation !== lifetime.version()) return;
    let current = true;
    // Refresh the inventory first so the replacement page finds its new ID.
    void client.invalidateQueries({ queryKey: queryKeys.containers }).finally(() => {
      if (!current || generation !== lifetime.version()) return;
      void navigate(containerHref(target));
      navigated.current?.();
    });
    return () => { current = false; };
  }, [target, navigate, lifetime, generation, client]);
  return <div role="status" className="flex flex-col gap-2 text-sm">
    <p>{run ? ta(`status.${run.status}`) : t("following")}</p>
    {run?.steps.map((step, index) => <p key={`${step.key}:${index}`}>{stepText(step.label)} · {ta(`step.${step.state}`)}</p>)}
    {run?.error && <p className="text-destructive">{run.error}</p>}
    {state.status === "error" && <p>{t("readFailed")}</p>}
    {run && <RecreateResultLink run={run} />}
  </div>;
}
