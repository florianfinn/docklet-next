// Source: docklet-next/issues/315
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original receipt-driven recreate progress and replacement navigation.

import { useEffect, useState } from "react";
import { Link, useNavigate } from "react-router";
import { useLocale, useTranslations } from "use-intl";
import type { ActionDetailState } from "@/data/adapter";
import { renderRun } from "@/data/message-code";
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

export function RecreateRun({ hostId, runId }: { hostId: string; runId: string }) {
  const adapter = useAdapter();
  const navigate = useNavigate();
  const locale = useLocale();
  const t = useTranslations("containers.recreate");
  const ta = useTranslations("activity");
  const [state, setState] = useState<ActionDetailState>({ status: "loading" });
  useEffect(() => {
    let current = true;
    const off = adapter.observeActionDetail?.(hostId, runId, (value) => { if (current) setState(value); });
    return () => { current = false; off?.(); };
  }, [adapter, hostId, runId]);
  const coded = state.status === "ready" || state.status === "error" ? state.run : undefined;
  const run = coded ? renderRun(coded, locale) : undefined;
  useEffect(() => {
    if (run?.status === "succeeded" && run.recreateResult?.newId) {
      const id = run.containerId?.includes("~") ? inventoryId(hostId, run.recreateResult.newId) : run.recreateResult.newId;
      void navigate(containerHref(id));
    }
  }, [run, hostId, navigate]);
  return <div role="status" className="flex flex-col gap-2 text-sm">
    <p>{run ? ta(`status.${run.status}`) : t("following")}</p>
    {run?.steps.map((step, index) => <p key={`${step.key}:${index}`}>{step.label} · {ta(`step.${step.state}`)}</p>)}
    {run?.error && <p className="text-destructive">{run.error}</p>}
    {state.status === "error" && <p>{t("readFailed")}</p>}
    {run && <RecreateResultLink run={run} />}
  </div>;
}
