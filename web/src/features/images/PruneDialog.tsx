// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230.

import { useState } from "react";
import { toast } from "sonner";
import { useTranslations } from "use-intl";
import type { Host } from "@/domain/model";
import { imageReference, shortDigest, pruneCandidates, totalSizeMb, type DockerImage, type ImagePruneResult, type PruneMode } from "@/domain/images";
import { formatMem } from "@/domain/format";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { Button } from "@/shared/ui/button";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { useImageCommands } from "./use-image-commands";
import { useImageHostCapabilities } from "./use-image-capabilities";
import { useImageError } from "./use-image-error";

export function PruneDialog({ open, onOpenChange, hosts, images }: { open: boolean; onOpenChange: (open: boolean) => void; hosts: Host[]; images: DockerImage[] }) {
  const t = useTranslations("images.pruneDialog"), error = useImageError(), cmd = useImageCommands();
  const [mode, setMode] = useState<PruneMode>("dangling"), [report, setReport] = useState<{ result: ImagePruneResult; images: DockerImage[]; hosts: Host[] }>();
  const scope = hosts.map((h) => h.id).sort().join("|");
  const feedback = useDialogCommand(open, scope, error);
  const capabilities = useImageHostCapabilities("pruneImages", hosts.map((h) => h.id));
  const connected = hosts.filter((h) => h.status === "online" && capabilities.get(h.id));
  const unavailable = hosts.filter((h) => h.status === "online" && !capabilities.get(h.id));
  const offline = hosts.filter((h) => h.status !== "online");
  const scoped = images.filter((i) => connected.some((h) => h.id === i.hostId));
  const remove = pruneCandidates(scoped, mode), kept = scoped.filter((i) => i.consumers.length);
  const change = (next: boolean) => { feedback.change(next, onOpenChange); if (!next) setReport(undefined); };
  const prune = useBusyAction(async () => {
    // Retain names even when a successful cleanup invalidates the current inventory.
    const snapshot = { images: [...images], hosts: [...hosts] };
    const result = await feedback.run(() => cmd.prune(connected.map((h) => h.id), mode), (result) => {
      if (!result) return;
      const message = result.hosts.map((host) => `${snapshot.hosts.find((h) => h.id === host.hostId)?.name ?? host.hostId}: ${host.errorCode ? error(host.errorCode) : [t("removed", { count: host.removed.length }), t("skipped", { count: host.skipped.length }), t("failed", { count: host.failed.length })].join(" · ")}`).join("; ");
      if (result.hosts.some((host) => host.errorCode || host.failed.length)) toast.error(message);
      else toast.success(message);
    });
    if (result) setReport({ ...snapshot, result });
  });
  return <Dialog open={open} onOpenChange={change}><DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-xl" data-surface="prune-dialog">
    <DialogHeader><DialogTitle>{t(report ? "report" : "title")}</DialogTitle><DialogDescription>{t("description")}</DialogDescription></DialogHeader>
    {report ? <div className="flex flex-col gap-4" role="status">{report.result.hosts.map((result) => <section key={result.hostId} className="rounded-lg border p-3 text-sm"><h3 className="font-medium">{report.hosts.find((h) => h.id === result.hostId)?.name ?? result.hostId}</h3>
      {result.errorCode ? <p role="alert">{error(result.errorCode)}</p> : <>
      <p>{t("removed", { count: result.removed.length })}</p><p>{t("skipped", { count: result.skipped.length })}</p><p>{t("failed", { count: result.failed.length })}</p>
      <p>{result.spaceReclaimed === undefined ? t("spaceUnknown") : t("space", { size: formatMem(result.spaceReclaimed / 1024 ** 2) })}</p>
      <ul>{result.removed.map((id) => <li key={id} className="break-all">{reportName(id, report.images)}</li>)}</ul>
      <ul>{[...result.skipped, ...result.failed].map((item) => <li key={`${item.id}-${item.code}`} className="break-all">{reportName(item.id, report.images)}: {error(item.code)}</li>)}</ul>
      </>}
    </section>)}</div> : <fieldset disabled={prune.busy} className="flex flex-col gap-4 text-sm">
      <p>{connected.length ? t("hosts", { hosts: connected.map((h) => h.name).join(", ") }) : t("hostsNone")}</p>
      {offline.length > 0 && <p>{t("skippedOffline", { hosts: offline.map((h) => h.name).join(", ") })}</p>}
      {unavailable.length > 0 && <p>{t("skippedUnavailable", { hosts: unavailable.map((h) => h.name).join(", ") })}</p>}
      <SegmentedToggle<PruneMode> value={mode} onChange={setMode} label={t("mode")} options={(["dangling", "unused"] as const).map((value) => ({ value, label: t(`modes.${value}`) }))} />
      <p>{t(`modeHint.${mode}`)}</p><p className="text-muted-foreground">{t("previewHint")}</p>
      <div data-slot="prune-preview"><p>{t("remove", { count: remove.length, size: formatMem(totalSizeMb(remove)) })}</p><ul>{remove.map((i) => <li key={i.id}>{imageReference(i)}</li>)}</ul>{kept.length > 0 && <p>{t("keep", { count: kept.length })}</p>}</div>
    </fieldset>}
    <DialogCommandFooter feedback={feedback}><Button variant="outline" onClick={() => change(false)}>{t(report ? "close" : "cancel")}</Button>{!report && <BusyButton variant="destructive" disabled={!connected.length} busy={prune.busy} onClick={() => void prune.run()}>{t("confirm")}</BusyButton>}</DialogCommandFooter>
  </DialogContent></Dialog>;
}

function reportName(id: string, images: DockerImage[]): string {
  const image = images.find((image) => image.id === id);
  return image?.tag ? imageReference(image) : shortDigest(image?.digest || id.slice(id.indexOf("~") + 1));
}
