// Source: docklet-next/docs/design/storage-workspace.md
// Revision: 1e03c054f5d2c355759cb79cbedfaf97706d4be1
// Retrieved: 2026-10-22
// License: Apache-2.0
// Deviations: Original volume rename dialog for issue #181.

import { useState } from "react";
import { useNavigate } from "react-router";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import { volumeName } from "@/domain/validation";
import type { Volume } from "@/domain/volumes";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { useVolumeCapability } from "./use-volume-capability";
import { useVolumeCommands } from "./use-volume-commands";

export function RenameVolumeDialog({ volume, onClose }: { volume: Volume; onClose: () => void }) {
  const t = useTranslations("volumes.renameDialog");
  const cmd = useVolumeCommands();
  const capability = useVolumeCapability("renameVolume", volume.id);
  const navigate = useNavigate();
  const [name, setName] = useState(volume.name);
  const [busy, setBusy] = useState(false);
  const invalid = !!volumeName(name, [volume.name]);
  return <Dialog open onOpenChange={(open) => { if (!open) onClose(); }}>
    <DialogContent>
      <form className="flex flex-col gap-4" onSubmit={async (event) => {
        event.preventDefault();
        if (busy || invalid || !capability.available) return;
        setBusy(true);
        const renamed = await cmd.renameVolume(volume.id, name.trim());
        setBusy(false);
        if (!renamed) return;
        toast.success(t("done", { name: renamed.name }));
        onClose();
        navigate(`/volumes/${encodeURIComponent(renamed.id)}`, { replace: true });
      }}>
        <DialogHeader><DialogTitle>{t("title")}</DialogTitle><DialogDescription>{t("description", { name: volume.name })}</DialogDescription></DialogHeader>
        <TextField id="rename-volume-name" label={t("name")} value={name} onChange={setName} description={t("hint")} mono />
        <DialogFooter>
          <Button type="button" variant="outline" onClick={onClose}>{t("cancel")}</Button>
          <Button type="submit" disabled={busy || invalid || !capability.available} title={capability.reason}>{t("submit")}</Button>
        </DialogFooter>
      </form>
    </DialogContent>
  </Dialog>;
}
