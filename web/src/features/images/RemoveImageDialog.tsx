// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useState } from "react";
import { toast } from "sonner";
import { useAdapter } from "@/data/use-adapter";
import { captureCommandFailure } from "@/shared/components/command-failure";
import { useTranslations } from "use-intl";
import { useCapability } from "@/data/use-capability";
import { imageReference, imageRemovalBlocked, type DockerImage } from "@/domain/images";
import { ConfirmDialog } from "@/shared/components/confirm-dialog";
import { Checkbox } from "@/shared/ui/checkbox";
import { Label } from "@/shared/ui/label";
import { useImageCommands } from "./use-image-commands";
export function RemoveImageDialog({ image, host, onClose, onRemoved }: { image: DockerImage; host: string; onClose: () => void; onRemoved: () => void }) {
  const t = useTranslations("images.removeDialog"), messages = useTranslations("images.toast"), cmd = useImageCommands(), adapter = useAdapter();
  const capability = useCapability("removeImage", { kind: "image", id: image.id });
  const [force, setForce] = useState(false);
  return <ConfirmDialog open onOpenChange={(open) => { if (!open) onClose(); }} title={t("title", { name: imageReference(image) })} description={t("body", { host })} confirmLabel={t("confirm")} destructive onDetached={(result) => { if (result === true) toast.success(messages("removed", { name: imageReference(image) })); }} onConfirm={async () => {
    if (!capability.available || imageRemovalBlocked(image)) return false;
    const current = captureCommandFailure(adapter).current;
    if (!await cmd.remove(image.id, imageReference(image), force)) return false;
    if (current()) onRemoved(); return true;
  }}><div className="flex items-center gap-2"><Checkbox id="image-force" checked={force} onCheckedChange={(value) => setForce(value === true)} /><Label htmlFor="image-force">{t("force")}</Label></div></ConfirmDialog>;
}
