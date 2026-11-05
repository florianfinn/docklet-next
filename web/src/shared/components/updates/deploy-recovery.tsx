// Source: docklet-next/issues/344
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original evidence-bound manual deployment recovery offer.

import { hasDeployRecovery } from "@/domain/deploy-recovery";
import { useImageError } from "@/features/images/use-image-error";
import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { ActionRun } from "@/domain/model";
import { Button } from "@/shared/ui/button";
import { StackUpdateDialog } from "./stack-update-dialog";

function RecoveryOffer({ run }: { run: ActionRun }) {
  const adapter = useAdapter(), t = useTranslations("common.stackUpdates"), [open, setOpen] = useState(false);
  const read = useCapability("listRestorePoints", { kind: "stack", id: run.stackId! });
  const restore = useCapability("restoreRestorePoint", { kind: "stack", id: run.stackId! });
  const query = useQuery({ queryKey: ["stack", run.stackId, "update-points"], queryFn: () => adapter.listRestorePoints!(run.stackId!), enabled: read.available && !!adapter.listRestorePoints });
  const point = query.data?.find((p) => p.id === run.restorePointId);
  return <div className="min-w-0"><Button variant="outline" disabled={!read.available || !restore.available || query.isError || !point?.available || point.orphaned || point.holdState !== "none"} title={read.reason ?? restore.reason} onClick={() => setOpen(true)}>{t("previousDeployment")}</Button>
    {open && point && <StackUpdateDialog key={`${run.id}:${point.id}`} stackId={run.stackId!} point={point} onClose={() => setOpen(false)} />}
  </div>;
}
export function DeployRecovery({ run }: { run: ActionRun }) {
  const t = useTranslations("common.stackUpdates"), errorText = useImageError();
  return <>{run.pointCode && <p role="status" className="text-xs text-muted-foreground"><span>{t("pointHint")}</span> {errorText(run.pointCode)}</p>}{hasDeployRecovery(run) && <RecoveryOffer key={`${run.id}:${run.restorePointId}`} run={run} />}</>;
}
