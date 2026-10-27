// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useState } from "react";
import { toast } from "sonner";
import { formatDateTime } from "@/domain/format";
import { useTranslations } from "use-intl";
import { useAdapter } from "@/data/use-adapter";
import { useCapability } from "@/data/use-capability";
import type { Registry, RegistryTest } from "@/domain/registries";
import { useImageError } from "@/features/images/use-image-error";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";

export function RegistryTestDialog({ registry, onClose }: { registry: Registry; onClose: () => void }) {
  const t = useTranslations("registries"), error = useImageError(), adapter = useAdapter();
  const feedback = useDialogCommand(true, registry.id, error);
  const capability = useCapability("testContainerRegistry", { kind: "registry", id: registry.id });
  const [result, setResult] = useState<RegistryTest>();
  const test = useBusyAction(async () => {
    if (!capability.available) return;
    setResult(undefined);
    const result = await feedback.run(() => adapter.testContainerRegistry(registry.id), (result) => { if (result) { const message = `${registry.url}: ${result.ok ? t("testSuccess") : error(result.code)}`; if (result.ok) toast.success(message); else toast.error(message); } });
    if (result) setResult(result);
  });
  return <Dialog open onOpenChange={(open) => { if (!open) feedback.change(false, onClose); }}><DialogContent {...feedback.bindings}>
    <DialogHeader><DialogTitle>{t("testTitle")}</DialogTitle><DialogDescription>{t("testHint")}</DialogDescription></DialogHeader>
    <p className="break-all text-sm">{registry.url}</p>
    {result && <div role={result.ok ? "status" : "alert"} className="text-sm"><p>{result.ok ? t("testSuccess") : error(result.code)}</p><p>{t("checkedAt", { time: formatDateTime(result.checkedAt) })}</p></div>}
    <DialogCommandFooter feedback={feedback}><Button variant="outline" onClick={() => feedback.change(false, onClose)}>{t("close")}</Button><BusyButton busy={test.busy} disabled={!capability.available} title={capability.reason} onClick={() => void test.run()}>{t("test")}</BusyButton></DialogCommandFooter>
  </DialogContent></Dialog>;
}
