// Source: docklet-next/issues/568
// Revision: 6ff4ef6a5b6a41d3ee3607b0360b5cf259b2b318
// Retrieved: 2026-11-22
// License: Apache-2.0
// Deviations: Original no-proof baseline action using ordinary no-pull deploy admission.; M36 #568: record baseline without pulling images.

import { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useTranslations } from "use-intl";
import { toast } from "sonner";

import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { Host, StackDetail } from "@/domain/model";
import { serviceRuntime } from "@/domain/status";
import { useImageError } from "@/features/images/use-image-error";
import { startPresented } from "@/features/activity/run-presentation";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Button } from "@/shared/ui/button";

/** Offered only while every service runs: recording uses `up`, which would otherwise start stopped services. */
export function RecordBaseline({ stack, host, disabled, inline }: { stack: StackDetail; host: Host | undefined; disabled: boolean; inline: boolean }) {
  const adapter = useAdapter(), t = useTranslations("stacks.detail"), errorText = useImageError();
  const read = useCapability("getUpdateBaseline", { kind: "stack", id: stack.id });
  const [confirm, setConfirm] = useState(false);
  const baseline = useQuery({ queryKey: ["stack", stack.id, "update-baseline"], queryFn: () => adapter.getUpdateBaseline!(stack.id), enabled: read.available && !!adapter.getUpdateBaseline });
  const noProof = baseline.data?.code === "update.baseline-unavailable" && !baseline.data.sourceRunId;
  const running = stack.services.length > 0 && stack.services.every((service) => ["running", "unhealthy"].includes(serviceRuntime(service, host)));
  if (!read.available || baseline.isError || !noProof || !running) return null;
  return <div className="flex flex-wrap items-center gap-3 text-sm">
    <span role="status">{t("noBaseline")}</span>
    <Button size="sm" variant="outline" disabled={disabled} onClick={() => setConfirm(true)}>{t("recordBaseline")}</Button>
    <ConfirmDialog open={confirm} onOpenChange={setConfirm} title={t("recordBaseline")} description={t("recordBaselineConfirm")} confirmLabel={t("recordBaseline")} formatError={errorText}
      onConfirm={async () => {
        if (disabled || !noProof) return false;
        const start = () => adapter.runStackCommand(stack.id, "start", { pullPolicy: "never" });
        const id = await (inline ? startPresented({ stackId: stack.id }, "inline", start) : start());
        if (!id) return false;
        toast.info(t("recordBaselineStarted"));
      }} />
  </div>;
}
