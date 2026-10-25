// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useTranslations } from "use-intl";
import type { DockerImage } from "@/domain/images";
import { ToneBadge } from "@/shared/components/tone";
import { useImageError } from "./use-image-error";

/** Read-only executor observation: unknown and failed checks never become current. */
export function ImageUpdate({ image, detail = false }: { image: DockerImage; detail?: boolean }) {
  const t = useTranslations("images.observation"), error = useImageError();
  const update = image.updateObservation;
  if (!update) return null;
  return <div className="flex flex-col gap-1 text-xs" data-image-update={update.state}>
    <ToneBadge tone={update.state === "current" ? "ok" : update.state === "version" || update.state === "rebuild" ? "warn" : "gray"}>{t(update.state)}</ToneBadge>
    {detail && <>
      {update.currentVersion && <span>{t("currentVersion", { version: update.currentVersion })}</span>}
      {update.candidateVersion && <span>{t("candidateVersion", { version: update.candidateVersion })}</span>}
      {update.checkedAt && <span>{t("checkedAt", { time: update.checkedAt })}</span>}
      {update.code && <span>{error(update.code)}</span>}
    </>}
  </div>;
}
