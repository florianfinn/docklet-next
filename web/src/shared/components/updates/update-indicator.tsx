import { useTranslations } from "use-intl";

import type { Service, UpdateInfo } from "@/domain/model";
import { summarizeUpdates } from "@/domain/updates";

import { InteractiveHoverCard } from "../interactive-hover-card";
import { StackUpdateChip, StackUpdatePanel, UpdateChip, UpdatePanel, type UpdateActions } from "./update-panel";
import type { UpdateTimes } from "./update-times";
import { useUpdateCopy } from "./update-visuals";

/** Update chip with a rich, keyboard-reachable hover card for one image. */
export function UpdateIndicator({ name, update, actions, side }: { name: string; update: UpdateInfo; actions?: UpdateActions; side?: "top" | "bottom" | "left" | "right" }) {
  const t = useTranslations("common.updates");
  const copy = useUpdateCopy()(update);
  return (
    <InteractiveHoverCard label={t("triggerLabel", { name, state: copy.title })} side={side} content={<UpdatePanel update={update} actions={actions} />}>
      <UpdateChip update={update} />
    </InteractiveHoverCard>
  );
}

export function StackUpdateIndicator({
  name,
  services,
  actions,
  side,
  times,
}: {
  name: string;
  services: Pick<Service, "name" | "update">[];
  actions?: UpdateActions;
  side?: "top" | "bottom" | "left" | "right";
  times?: UpdateTimes;
}) {
  const t = useTranslations("common.updates");
  const s = summarizeUpdates(services);
  const state = s.pending ? t("chipPending", { count: s.pending }) : s.errors ? t("chipError") : s.visual === "unknown" ? t("chipUnknown") : s.visual === "unchecked" ? t("chipUnchecked") : s.visual === "local" ? t("chipLocal") : t("chipCurrent");
  return (
    <InteractiveHoverCard label={t("triggerLabel", { name, state })} side={side} content={<StackUpdatePanel services={services} actions={actions} times={times} />}>
      <StackUpdateChip services={services} />
    </InteractiveHoverCard>
  );
}
