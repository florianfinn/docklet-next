// Source: docklet-next/issues/316
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original per-stack attention service capability and restart gesture.

import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import { requestServiceRestart } from "@/shared/components/service-restart-store";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { Button } from "@/shared/ui/button";
import type { AttentionItem } from "../attention";

export function AttentionServiceRestart({ item }: { item: AttentionItem }) {
  const restart = useCapability("restartService", { kind: "stack", id: item.stackId ?? "" });
  const cmd = useStackCommands();
  const t = useTranslations("dashboard.attention.actions");
  return <Button size="sm" variant="outline" className="h-7 px-2 text-xs" disabled={!restart.available || !item.stackId || !item.service} title={restart.reason} onClick={() => {
    const { stackId, service } = item;
    if (stackId && service) requestServiceRestart([{ stackId, stackName: String(item.values.stack ?? stackId), service, containerId: item.containerId }], () => cmd.restartService(stackId, service));
  }}>{t("restart")}</Button>;
}
