import { useImageSecurity } from "./use-image-security";
import { useCapability } from "@/data/use-capability";
import { Download, Eye, Layers, ShieldAlert, Trash2 } from "lucide-react";
import { useNavigate } from "react-router";
import { useTranslations } from "use-intl";

import { imageRemovalBlocked, type DockerImage, type ImageUsage } from "@/domain/images";
import type { Host, Tone } from "@/domain/model";
import { hasUpdate } from "@/domain/updates";
import type { MenuGroups } from "@/shared/components/action-menu";

export const USAGE_TONE: Record<ImageUsage, Tone> = { "in-use": "ok", unused: "warn", dangling: "orange", unknown: "gray" };

/** Actions of one image; the ellipsis button and the row's context menu share them. */
export function useImageMenu(image: DockerImage, host: Host | undefined, open: { details: (id: string) => void; pull: (image: DockerImage) => void; remove: (image: DockerImage) => void }): MenuGroups {
  const t = useTranslations("images.menu");
  const navigate = useNavigate();
  const security = useImageSecurity(image);
  const pull = useCapability("pullImage", { kind: "host", id: image.hostId });
  const remove = useCapability("removeImage", { kind: "image", id: image.id });
  const online = host?.status === "online";
  const stack = image.consumers.find((c) => c.stackId && !c.system)?.stackId;
  return [
    [{ id: "details", label: t("details"), icon: <Eye />, onSelect: () => open.details(image.id) }],
    [
      { id: "pull", label: t("pullAgain"), icon: <Download />, disabled: !pull.available || !!image.inventory?.stale || !online || image.tag === null, title: pull.reason, onSelect: () => open.pull(image) },
      ...(image.update && hasUpdate(image.update) && stack ? [{ id: "update", label: t("openUpdate"), icon: <Layers />, onSelect: () => navigate(`/stacks/${stack}`) }] : []),
      ...(security.counts || security.unavailable || security.notScanned ? [{ id: "security", label: t("security"), icon: <ShieldAlert />, onSelect: () => navigate("/security?tab=images") }] : []),
    ],
    [{ id: "remove", label: t("remove"), icon: <Trash2 />, disabled: !remove.available || !!image.inventory?.stale || !online || imageRemovalBlocked(image), title: remove.reason, onSelect: () => open.remove(image) }],
  ];
}
