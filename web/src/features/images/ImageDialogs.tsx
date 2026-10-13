import { Download, Eraser } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { formatMem } from "@/domain/format";
import { imageIdFor, imageReference, parseImageReference, pruneCandidates, totalSizeMb, type DockerImage, type PruneMode } from "@/domain/images";
import type { Host } from "@/domain/model";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { TextField } from "@/shared/components/form-field";
import { SegmentedToggle } from "@/shared/components/segmented-toggle";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useImageCommands } from "./use-image-commands";

const SUGGESTIONS = ["redis:7.4-alpine", "postgres:17-alpine", "traefik/whoami:v1.11", "ghcr.io/example/app:1.2.0"];

/** Pull form: target host (connected only), validated reference, optional demo failure. */
export function PullDialog({ open, onOpenChange, hosts, images, initial }: { open: boolean; onOpenChange: (o: boolean) => void; hosts: Host[]; images: DockerImage[]; initial?: { hostId: string; reference: string } }) {
  const t = useTranslations("images.pullDialog");
  const cmd = useImageCommands();
  const firstOnline = hosts.find((h) => h.status === "online")?.id ?? "";
  const [hostId, setHostId] = useState(initial?.hostId ?? firstOnline);
  const [reference, setReference] = useState(initial?.reference ?? "");
  const [fail, setFail] = useState(false);
  const [touched, setTouched] = useState(false);
  const [seen, setSeen] = useState(open);
  if (open !== seen) {
    setSeen(open);
    if (open) {
      setHostId(initial?.hostId ?? firstOnline);
      setReference(initial?.reference ?? "");
      setFail(false);
      setTouched(false);
    }
  }
  const parsed = parseImageReference(reference);
  const known = parsed ? images.some((i) => i.id === imageIdFor(hostId, parsed.repository, parsed.tag)) : false;
  const host = hosts.find((h) => h.id === hostId);
  const valid = !!parsed && host?.status === "online";

  // Busy only for the request; the pull itself continues as an action toast after the dialog closes.
  const pull = useBusyAction(async () => {
    const id = await cmd.pull({ hostId, reference: reference.trim() }, fail);
    if (id) onOpenChange(false);
  });
  const submit = async () => {
    setTouched(true);
    if (!valid) return;
    await pull.run();
  };

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-lg" data-surface="pull-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <form
          id="pull-image"
          className="flex flex-col gap-4"
          onSubmit={(e) => {
            e.preventDefault();
            void submit();
          }}
        >
          <div className="flex flex-col gap-2">
            <Label htmlFor="pull-host">{t("host")}</Label>
            <Select value={hostId} onValueChange={setHostId}>
              <SelectTrigger id="pull-host" className="w-full">
                <SelectValue />
              </SelectTrigger>
              <SelectContent>
                <SelectGroup>
                  {hosts.map((h) => (
                    <SelectItem key={h.id} value={h.id} disabled={h.status !== "online"}>
                      {h.status === "online" ? h.name : t("hostOffline", { name: h.name })}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <TextField id="pull-ref" label={t("reference")} value={reference} onChange={setReference} mono placeholder="redis:7.4-alpine" description={known ? t("known") : t("referenceHint")} error={(touched || reference.length > 2) && !parsed ? t("referenceInvalid") : undefined} />
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            <span className="text-muted-foreground">{t("suggestions")}</span>
            {SUGGESTIONS.map((s) => (
              <Button key={s} type="button" size="sm" variant="outline" className="h-6 px-2 font-mono text-[11px]" onClick={() => setReference(s)}>
                {s}
              </Button>
            ))}
          </div>
          <div className="flex items-center gap-2">
            <Checkbox id="pull-fail" checked={fail} onCheckedChange={(v) => setFail(v === true)} />
            <Label htmlFor="pull-fail" className="text-sm font-normal text-muted-foreground">
              {t("fail")}
            </Label>
          </div>
        </form>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <BusyButton type="submit" form="pull-image" disabled={!valid} busy={pull.busy} icon={<Download data-icon="inline-start" />}>
            {t("submit")}
          </BusyButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}

/** Prune preview: exactly what goes and what stays, per connected host in the filter. */
export function PruneDialog({ open, onOpenChange, hosts, images }: { open: boolean; onOpenChange: (o: boolean) => void; hosts: Host[]; images: DockerImage[] }) {
  const t = useTranslations("images.pruneDialog");
  const cmd = useImageCommands();
  const [mode, setMode] = useState<PruneMode>("dangling");
  const connected = hosts.filter((h) => h.status === "online");
  const offline = hosts.filter((h) => h.status !== "online");
  const scoped = images.filter((i) => connected.some((h) => h.id === i.hostId));
  const remove = pruneCandidates(scoped, mode);
  const kept = scoped.filter((i) => i.consumers.length > 0);
  const name = (i: DockerImage) => `${imageReference(i)} · ${connected.find((h) => h.id === i.hostId)?.name ?? i.hostId}`;
  const prune = useBusyAction(async () => {
    await cmd.prune(
      connected.map((h) => h.id),
      mode,
    );
    onOpenChange(false);
  });
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-xl" data-surface="prune-dialog">
        <DialogHeader>
          <DialogTitle>{t("title")}</DialogTitle>
          <DialogDescription>{t("description")}</DialogDescription>
        </DialogHeader>
        <div className="flex flex-col gap-4 text-sm">
          <p className="text-xs text-muted-foreground">{connected.length ? t("hosts", { hosts: connected.map((h) => h.name).join(", ") }) : t("hostsNone")}</p>
          {offline.length > 0 && <p className="text-xs text-warning">{t("skippedOffline", { hosts: offline.map((h) => h.name).join(", ") })}</p>}
          <div className="flex flex-col gap-1.5">
            <span className="text-sm font-medium">{t("mode")}</span>
            <SegmentedToggle<PruneMode> value={mode} onChange={setMode} label={t("mode")} options={(["dangling", "unused"] as const).map((m) => ({ value: m, label: t(`modes.${m}`) }))} />
            <p className="text-xs text-muted-foreground">{t(`modeHint.${mode}`)}</p>
          </div>
          <div className="rounded-xl border bg-muted/15 p-3" data-slot="prune-preview">
            <p className="font-medium">{t("remove", { count: remove.length, size: formatMem(totalSizeMb(remove)) })}</p>
            {remove.length > 0 && (
              <ul className="mt-2 max-h-40 overflow-auto font-mono text-xs">
                {remove.map((i) => (
                  <li key={i.id} className="flex justify-between gap-3 py-0.5">
                    <span className="[overflow-wrap:anywhere]">{name(i)}</span>
                    <span className="shrink-0 text-muted-foreground">{formatMem(i.sizeMb)}</span>
                  </li>
                ))}
              </ul>
            )}
            {kept.length > 0 && <p className="mt-2 text-xs text-success">{t("keep", { count: kept.length })}</p>}
          </div>
        </div>
        <DialogFooter>
          <Button type="button" variant="outline" onClick={() => onOpenChange(false)}>
            {t("cancel")}
          </Button>
          <BusyButton type="button" variant="destructive" disabled={!remove.length} busy={prune.busy} icon={<Eraser data-icon="inline-start" />} onClick={() => void prune.run()}>
            {t("confirm")}
          </BusyButton>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  );
}
