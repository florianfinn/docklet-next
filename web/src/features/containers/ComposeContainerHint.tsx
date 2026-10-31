// Source: docklet-next/issues/253
// Revision: 1e13c0e4d6fc576a464fa92b145063016c27a1c2
// Retrieved: 2026-10-31
// License: Apache-2.0
// Deviations: Original Compose-child alternatives using stack admission.

import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import type { ContainerInfo } from "@/domain/model";
import { useStackCommands } from "@/shared/hooks/use-stack-commands";
import { requestServiceRestart } from "@/shared/components/service-restart-store";
import { Alert, AlertDescription, AlertTitle } from "@/shared/ui/alert";
import { Button } from "@/shared/ui/button";

export function ComposeContainerHint({ c }: { c: ContainerInfo }) {
  const t = useTranslations("containers.composeHint");
  const cmd = useStackCommands();
  const ref = { kind: "stack", id: c.stackId ?? "" };
  const stack = useCapability("runStackCommand", ref);
  const restart = useCapability("restartService", ref);
  return <Alert><AlertTitle>{t("title")}</AlertTitle><AlertDescription className="flex flex-col gap-2">
    <p>{t("body")}</p>
    <div className="flex flex-wrap gap-2">
      <Button size="sm" variant="outline" disabled={!c.stackId || !stack.available || !!c.inventory?.stale} title={stack.reason} onClick={() => c.stackId && void cmd.run(c.stackId, "redeploy")}>{t("stack")}</Button>
      <Button size="sm" variant="outline" disabled={!c.stackId || !c.serviceName || !restart.available || !!c.inventory?.stale} title={restart.reason} onClick={() => {
        if (c.stackId && c.serviceName) { const stackId = c.stackId, service = c.serviceName; requestServiceRestart([{ stackId, service, containerId: c.id, stackName: c.stackName ?? stackId }], () => cmd.restartService(stackId, service)); }
      }}>{t("service")}</Button>
    </div>
  </AlertDescription></Alert>;
}
