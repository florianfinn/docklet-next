// Source: docklet-next/issues/265
// Revision: 89f69980c01d08714ce96c49365fb19065e3d6ba
// Retrieved: 2026-11-05
// License: Apache-2.0
// Deviations: Original container scan platform coverage hint.

import { useTranslations } from "use-intl";
import { useSecurity } from "@/data/queries";
import type { ContainerInfo } from "@/domain/model";
import { platformCoverage } from "@/features/security/platform-coverage";

/** Marks a container whose platform the image's last completed scan does not cover. */
export function ContainerScanCoverage({ c }: { c: ContainerInfo }) {
  const t = useTranslations("security.coverage");
  const security = useSecurity();
  const image = c.imageId ? security.data?.images.find((r) => r.id === c.imageId && r.hostId === c.hostId) : undefined;
  if (!image?.scannedAt || image.unavailable) return null;
  const state = platformCoverage(c.imagePlatform, image.platform);
  if (state === "not-covered") return <p className="text-xs text-warning">{t("containerNotCovered", { platform: c.imagePlatform ?? "", scanned: image.platform ?? "" })}</p>;
  if (state === "unknown") return <p className="text-xs text-muted-foreground">{t("containerUnknown")}</p>;
  return null;
}
