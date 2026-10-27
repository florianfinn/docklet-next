// Source: docklet-next/docs/design/vulnerability-scanning.md
// Revision: cf5aa27a8ca449d95c66dcd253f7a1a2e114f0f5
// Retrieved: 2026-10-27
// License: Apache-2.0
// Deviations: Original live vulnerability UI and contract regressions for issue #262.

import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import { useScanAction } from "@/features/security/use-scan-action";
import type { DockerImage } from "@/domain/images";
import { Button } from "@/shared/ui/button";

export function ScanImageButton({ image }: { image: DockerImage }) {
  const cap = useCapability("runScan", { kind: "image", id: image.id }), action = useScanAction(), t = useTranslations("security");
  return <div className="flex flex-col gap-1">
    <Button size="sm" variant="outline" disabled={!cap.available || action.busy} title={cap.reason} onClick={() => void action.start({ imageId: image.id })}>{t("scan")}</Button>
    {action.note && <p role="status">{action.note}</p>}
    {action.error && <p role="alert">{action.error}</p>}
  </div>;
}
