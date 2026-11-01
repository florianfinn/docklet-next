// Source: docklet-next/issues/323
// Revision: 6365cc5cebd04f199d15fe5f53c006a127d0a669
// Retrieved: 2026-11-01
// License: Apache-2.0
// Deviations: Original runtime-view container controls and shell within assigned stacks.

import { useState } from "react";
import { lazy, Suspense } from "react";
import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import { useAdapter } from "@/data/use-adapter";
import { useCommandError } from "@/data/use-command-error";
import type { Service, StackDetail } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/shared/ui/card";
import { captureCommandFailure } from "@/shared/components/command-failure";
const ContainerTerminal = lazy(() => import("@/features/containers/ContainerTerminal"));

export function UserContainers({ stack }: { stack: StackDetail }) {
  const t = useTranslations("shell.nav");
  return <Card><CardHeader><CardTitle>{t("containers")}</CardTitle></CardHeader><CardContent className="flex flex-col gap-3">{stack.services.filter((s) => s.containerId).map((s) => <UserContainer key={s.containerId} service={s} />)}</CardContent></Card>;
}
function UserContainer({ service }: { service: Service }) {
  const adapter = useAdapter();
  const t = useTranslations("stacks.detail.actions");
  const ts = useTranslations("containers.terminal");
  const errorText = useCommandError();
  const [shell, setShell] = useState(false);
  const [busy, setBusy] = useState(false);
  const id = service.containerId!;
  const command = useCapability("runContainerCommand", { kind: "container", id });
  const terminal = useCapability("openContainerShell", { kind: "container", id });
  return <section className="flex flex-col gap-2 rounded-lg border p-3"><span className="font-mono text-sm">{service.name}</span><div className="flex flex-wrap gap-2">
    {(["start", "stop", "restart"] as const).map((action) => <Button key={action} size="sm" variant="outline" disabled={busy || !command.available} title={command.reason} onClick={() => {
      const failure = captureCommandFailure(adapter); setBusy(true);
      void adapter.runContainerCommand(id, action).catch((e: unknown) => failure.error(errorText(e))).finally(() => setBusy(false));
    }}>{t(action)}</Button>)}
    <Button size="sm" variant="outline" disabled={!terminal.available || service.state !== "running"} title={terminal.reason} onClick={() => setShell((open) => !open)}>{ts("title")}</Button>
  </div>{shell && <Suspense fallback={null}><ContainerTerminal containerId={id} /></Suspense>}</section>;
}
