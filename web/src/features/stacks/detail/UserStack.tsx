// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original reduced stack details reusing the approved header, services, editor and log surfaces.

import { useState } from "react";
import { useSearchParams } from "react-router";
import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import { useHostScope } from "@/shared/hooks/use-host-scope";
import type { StackDetail } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { Card } from "@/shared/ui/card";
import { StackWorkspace, type EditorMode } from "../editor/StackWorkspace";
import { useComposeDraft } from "../editor/use-compose-draft";
import { StackLogs } from "../logs/StackLogs";
import { StackHeader } from "./StackHeader";
import { ServicesCard } from "./ServicesCard";
import { UserContainers } from "./UserContainers";

export function UserStack({ stack }: { stack: StackDetail }) {
  const t = useTranslations("stacks.detail");
  const te = useTranslations("editor");
  const [params, setParams] = useSearchParams();
  const edit = useCapability("saveStackFiles", { kind: "stack", id: stack.id });
  const tab = params.get("tab") === "logs" ? "logs" : params.get("tab") === "editor" && edit.available ? "editor" : "overview";
  const hosts = useHostScope();
  const [mode, setMode] = useState<EditorMode>("advanced");
  const draft = useComposeDraft({ compose: stack.compose, env: stack.env, hostId: stack.hostId, labels: stack.labels });
  return <>
    <StackHeader stack={stack} host={hosts.byId.get(stack.hostId)} />
    <nav aria-label={t("tabsLabel")} className="mb-4 flex flex-wrap gap-2">{(["overview", "editor", "logs"] as const).map((key) => <Button key={key} variant={key === tab ? "secondary" : "ghost"} size="sm" disabled={key === "editor" && !edit.available} title={key === "editor" ? edit.reason : undefined} onClick={() => setParams({ tab: key }, { replace: true })}>{t(`tabs.${key}`)}</Button>)}</nav>
    {tab === "logs" ? <StackLogs stack={stack} /> : tab === "editor" ? stack.filesUnavailable ? <Card className="p-4">{te("filesUnavailable")}</Card> : <StackWorkspace stack={stack} draft={draft} hosts={hosts.all} labelSuggestions={stack.labels} mode={mode} onMode={setMode} /> : <div className="flex flex-col gap-4"><ServicesCard stack={stack} host={hosts.byId.get(stack.hostId)} /><UserContainers stack={stack} /></div>}
  </>;
}
