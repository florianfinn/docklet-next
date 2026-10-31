// Source: docklet-next/issues/312
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original container stream tabs with authoritative shell availability.

import { useQuery } from "@tanstack/react-query";
import { lazy, Suspense, useId, type ReactNode } from "react";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import type { ContainerInfo, Host } from "@/domain/model";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/shared/ui/tabs";
import { Skeleton } from "@/shared/ui/skeleton";
import { shellMessage } from "./shell-message";
const ContainerTerminal = lazy(() => import("./ContainerTerminal"));

export function ContainerTerminalTabs({ c, host, children }: { c: ContainerInfo; host?: Host; children: ReactNode }) {
  const adapter = useAdapter();
  const t = useTranslations("containers.terminal");
  const tp = useTranslations("containers.page");
  const reasonId = useId();
  const capability = useQuery({
    queryKey: ["capability", "openContainerShell", c.id, c.state, c.inventory?.reason, host?.status, host?.agentVersion],
    queryFn: () => adapter.getCapability("openContainerShell", { kind: "container", id: c.id }),
    enabled: c.state === "running",
  });
  // The descriptor owns host availability, including standby; retained host
  // labels alone cannot establish whether an on-demand shell can open.
  const code = c.state !== "running" ? "container.shell-not-running" : capability.data?.disabledReasonCode;
  const disabled = c.state !== "running" || capability.data?.available !== true;
  const reason = capability.isPending && c.state === "running" ? t("checking") : t(shellMessage(code ?? "capability.unavailable"));
  return (
    <Tabs defaultValue="logs">
      <TabsList aria-label={tp("details")}>
        <TabsTrigger value="logs">{tp("logs")}</TabsTrigger>
        <TabsTrigger value="terminal" disabled={disabled} title={disabled ? reason : undefined} aria-describedby={disabled ? reasonId : undefined}>{t("title")}</TabsTrigger>
      </TabsList>
      {disabled && <p id={reasonId} className="text-sm text-muted-foreground">{reason}</p>}
      <TabsContent value="logs">{children}</TabsContent>
      <TabsContent value="terminal">
        {disabled ? <p role="status">{reason}</p> : <Suspense fallback={<Skeleton className="h-80 w-full" />}><ContainerTerminal key={c.id} containerId={c.id} /></Suspense>}
      </TabsContent>
    </Tabs>
  );
}
