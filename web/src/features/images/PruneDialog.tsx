// Source: docklet-next/docs/design/images-updates.md
// Revision: 4b115b750db13ed0934695adb071d6d4eb374310
// Retrieved: 2026-10-25
// License: Apache-2.0
// Deviations: Original production image and registry UI implementation for issue #230. M14 host maintenance and availability. M43 #661 per-image progress and compact counts.

import { isHostAvailable } from "@/domain/hosts";
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
import { NameDetails, PruneProgress, usePruneCountText, usePruneRunSnapshot, usePruneRuns } from "./prune-progress";

export function PruneDialog({ open, onOpenChange, hosts, images }: { open: boolean; onOpenChange: (open: boolean) => void; hosts: Host[]; images: DockerImage[] }) {
  const t = useTranslations("images.pruneDialog"), error = useImageError(), cmd = useImageCommands();
  const [mode, setMode] = useState<PruneMode>("dangling"), [report, setReport] = useState<{ result: ImagePruneResult; images: DockerImage[]; hosts: Host[] }>();
  const scope = hosts.map((h) => h.id).sort().join("|");
  const feedback = useDialogCommand(open, scope, error);
  const capabilities = useImageHostCapabilities("pruneImages", hosts.map((h) => h.id));
  const connected = hosts.filter((h) => isHostAvailable(h) && capabilities.get(h.id));
  const unavailable = hosts.filter((h) => isHostAvailable(h) && !capabilities.get(h.id));
  const offline = hosts.filter((h) => !isHostAvailable(h));
  const scoped = images.filter((i) => connected.some((h) => h.id === i.hostId));
  const remove = pruneCandidates(scoped, mode), kept = scoped.filter((i) => i.consumers.length);
  const [before, setBefore] = useState<ReadonlySet<string>>(), snapshotRuns = usePruneRunSnapshot(), counts = usePruneCountText();
  const [pending, setPending] = useState(false);
  const runs = usePruneRuns(before, (report?.hosts ?? connected).map((h) => h.id), pending);
  const change = (next: boolean) => { feedback.change(next, onOpenChange); if (!next) { setReport(undefined); setBefore(undefined); } };
  const prune = useBusyAction(async () => {
    // Retain names even when a successful cleanup invalidates the current inventory.
    const snapshot = { images: [...images], hosts: [...hosts] };
    setBefore(snapshotRuns()); setPending(true);
    const result = await feedback.run(() => cmd.prune(connected.map((h) => h.id), mode), (result) => {
      if (!result) return;
      // One compact summary: totals, plus the hosts whose cleanup had no result.
      const done = result.hosts.filter((host) => !host.errorCode);
      const totals = [t("removed", { count: sum(done, (h) => h.removed.length) }), t("skipped", { count: sum(done, (h) => h.skipped.length) }), t("failed", { count: sum(done, (h) => h.failed.length) })].join(" · ");
      const failures = result.hosts.filter((host) => host.errorCode).map((host) => `${snapshot.hosts.find((h) => h.id === host.hostId)?.name ?? host.hostId}: ${error(host.errorCode!)}`);
      const message = [totals, ...failures].join("; ");
      if (failures.length || done.some((host) => host.failed.length)) toast.error(message);
      else toast.success(message);
    });
    setPending(false);
    if (result) setReport({ ...snapshot, result });
  });
  return <Dialog open={open} onOpenChange={change}><DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-auto sm:max-w-xl" data-surface="prune-dialog">
    <DialogHeader><DialogTitle>{t(report ? "report" : "title")}</DialogTitle><DialogDescription>{t("description")}</DialogDescription></DialogHeader>
    {report ? <div className="flex flex-col gap-3" role="status">
      {report.result.hosts.length > 1 && <p className="font-medium" data-slot="prune-totals">{totalsText(report.result, t)}</p>}
      {report.result.hosts.map((result) => { const observed = runs.get(result.hostId)?.bulk; return <section key={result.hostId} className="flex flex-col gap-1 rounded-lg border p-3 text-sm"><h3 className="font-medium">{report.hosts.find((h) => h.id === result.hostId)?.name ?? result.hostId}</h3>
      {result.errorCode ? <><p role="alert">{error(result.errorCode)}</p>{observed && <p data-slot="prune-incomplete">{counts(observed, false)} · {t("incomplete")}</p>}</> : <>
      <p className="tabular-nums">{[t("removed", { count: result.removed.length }), t("skipped", { count: result.skipped.length }), t("failed", { count: result.failed.length })].join(" · ")}</p>
      <p className="text-muted-foreground">{result.spaceReclaimed === undefined ? t("spaceUnknown") : t("space", { size: formatMem(result.spaceReclaimed / 1024 ** 2) })}</p>
      {result.skipped.length + result.failed.length > 0 && <ul className="text-xs">{[...result.failed, ...result.skipped].map((item) => <li key={`${item.id}-${item.code}`} className="break-all">{reportName(item.id, report.images)}: {error(item.code)}</li>)}</ul>}
      <NameDetails count={result.removed.length}>{result.removed.map((id) => <li key={id} className="break-all">{reportName(id, report.images)}</li>)}</NameDetails>
      </>}
    </section>; })}</div> : prune.busy ? <PruneProgress hosts={connected} runs={runs} /> : <fieldset disabled={prune.busy} className="flex flex-col gap-4 text-sm">
      <p>{connected.length ? t("hosts", { hosts: connected.map((h) => h.name).join(", ") }) : t("hostsNone")}</p>
      {offline.length > 0 && <p>{t("skippedOffline", { hosts: offline.map((h) => h.name).join(", ") })}</p>}
      {unavailable.length > 0 && <p>{t("skippedUnavailable", { hosts: unavailable.map((h) => h.name).join(", ") })}</p>}
      <SegmentedToggle<PruneMode> value={mode} onChange={setMode} label={t("mode")} options={(["dangling", "unused"] as const).map((value) => ({ value, label: t(`modes.${value}`) }))} />
      <p>{t(`modeHint.${mode}`)}</p><p className="text-muted-foreground">{t("previewHint")}</p>
      <div data-slot="prune-preview" className="flex flex-col gap-1"><p className="font-medium">{t("remove", { count: remove.length, size: formatMem(totalSizeMb(remove)) })}</p>{kept.length > 0 && <p className="text-muted-foreground">{t("keep", { count: kept.length })}</p>}<NameDetails count={remove.length}>{remove.map((i) => <li key={i.id}>{imageReference(i)}</li>)}</NameDetails></div>
    </fieldset>}
    <DialogCommandFooter feedback={feedback}><Button variant="outline" onClick={() => change(false)}>{t(report ? "close" : "cancel")}</Button>{!report && <BusyButton variant="destructive" disabled={!connected.length} busy={prune.busy} onClick={() => void prune.run()}>{t("confirm")}</BusyButton>}</DialogCommandFooter>
  </DialogContent></Dialog>;
}

const sum = <T,>(items: T[], count: (item: T) => number) => items.reduce((n, item) => n + count(item), 0);

function totalsText(result: ImagePruneResult, t: ReturnType<typeof useTranslations<"images.pruneDialog">>): string {
  const done = result.hosts.filter((host) => !host.errorCode);
  return t("totals", { removed: sum(done, (h) => h.removed.length), skipped: sum(done, (h) => h.skipped.length), failed: sum(done, (h) => h.failed.length), hosts: result.hosts.length });
}

function reportName(id: string, images: DockerImage[]): string {
  const image = images.find((image) => image.id === id);
  return image?.tag ? imageReference(image) : shortDigest(image?.digest || id.slice(id.indexOf("~") + 1));
}
