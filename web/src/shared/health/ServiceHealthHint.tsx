// Source: docklet-next/issues/637
// Revision: 24638d9cd8ce431af9ee9b829ec68b171c77e287
// Retrieved: 2026-11-26
// License: Apache-2.0
// Deviations: Original M42 lazy service health hints and admin deep links. M42 #637: shared hint text and one-sentence probe line.

import { useState } from "react";
import { Link } from "react-router";
import { useTranslations } from "use-intl";
import { useContainerHealth } from "@/data/queries";
import { diagnoseHealth } from "@/domain/health";
import type { Service, Tone } from "@/domain/model";
import { InteractiveHoverCard } from "@/shared/components/interactive-hover-card";
import { ToneBadge } from "@/shared/components/tone";
import { useHealthHint } from "./health-hint";

/** The caller supplies admin navigation context; closed previews never poll. */
export function ServiceHealthHint({ containerId, stackId, serviceName, open }: {
  containerId?: string; stackId: string; serviceName: string; open: boolean;
}) {
  const t = useTranslations("health");
  const health = useContainerHealth(containerId, open);
  const hint = useHealthHint()(health.data ? diagnoseHealth(health.data, health.dataUpdatedAt) : undefined);
  return <div className="grid gap-2 text-xs">
    {!containerId ? <p>{t("noContainer")}</p> : health.isError ? <p role="alert">{t("unavailable")}</p> : hint ? <p>{hint}</p> : health.isPending ? <p>{t("loading")}</p> : null}
    <Link className="text-primary-text underline-offset-4 hover:underline" to={`/stacks/${encodeURIComponent(stackId)}?tab=health&service=${encodeURIComponent(serviceName)}`}>{t("showHealth")}</Link>
  </div>;
}

export function ServiceHealthBadge({ service, stackId, tone, label }: {
  service: Service; stackId: string; tone: Tone; label: string;
}) {
  const [open, setOpen] = useState(false);
  return <InteractiveHoverCard label={`${service.name}: ${label}`} onOpenChange={setOpen}
    content={<div className="p-3"><ServiceHealthHint containerId={service.containerId} stackId={stackId} serviceName={service.name} open={open} /></div>}>
    <ToneBadge tone={tone}>{label}</ToneBadge>
  </InteractiveHoverCard>;
}
