export { PruneDialog } from "./PruneDialog";
import { useCapability } from "@/data/use-capability";
import { useRegistries } from "@/features/registries/use-registries";
import { registryHost, registryReference } from "@/domain/registries";
import { useImageHostCapabilities } from "./use-image-capabilities";
import { DialogCommandFooter } from "@/shared/components/dialog-command";
import { useDialogCommand } from "@/shared/components/use-dialog-command";
import { Download } from "lucide-react";
import { useState } from "react";
import { useTranslations } from "use-intl";

import { parseImageReference, type DockerImage, type ImagePullProgress } from "@/domain/images";
import type { Host } from "@/domain/model";
import { BusyButton } from "@/shared/components/busy-button";
import { useBusyAction } from "@/shared/components/use-busy-action";
import { TextField } from "@/shared/components/form-field";
import { Button } from "@/shared/ui/button";
import { Checkbox } from "@/shared/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/shared/ui/dialog";
import { Label } from "@/shared/ui/label";
import { Select, SelectContent, SelectGroup, SelectItem, SelectTrigger, SelectValue } from "@/shared/ui/select";

import { useImageCommands } from "./use-image-commands";

const SUGGESTIONS = ["redis:7.4-alpine", "postgres:17-alpine", "traefik/whoami:v1.11", "ghcr.io/example/app:1.2.0"];

/** Pull targets an allowed connected host and resolves saved credentials by registry host. */
export function PullDialog({ open, onOpenChange, hosts, images, initial }: { open: boolean; onOpenChange: (o: boolean) => void; hosts: Host[]; images: DockerImage[]; initial?: { hostId: string; reference: string } }) {
  const t = useTranslations("images.pullDialog");
  const cmd = useImageCommands();
  const firstOnline = hosts.find((h) => h.status === "online")?.id ?? "";
  const [hostId, setHostId] = useState(initial?.hostId ?? firstOnline);
  const [registryId, setRegistryId] = useState("auto");
  const feedback = useDialogCommand(open, `${hostId}|${registryId}`);
  const change = (next: boolean) => feedback.change(next, onOpenChange);
  const [reference, setReference] = useState(initial?.reference ?? "");
  const [fail, setFail] = useState(false);
  const [progress, setProgress] = useState<{ owner: string; value: ImagePullProgress }>();
  const [touched, setTouched] = useState(false);
  const [seen, setSeen] = useState(open);
  if (open !== seen) {
    setSeen(open);
    if (open) {
      setHostId(initial?.hostId ?? firstOnline);
      setReference(initial?.reference ?? "");
      setFail(false);
      setProgress(undefined);
      setRegistryId("auto");
      setTouched(false);
    }
  }
  const registries = useRegistries(open);
  const selectedRegistry = registries.data?.find((r) => r.id === registryId && r.enabled);
  const effectiveReference = registryReference(reference, selectedRegistry && registryHost(selectedRegistry.url));
  const capabilities = useImageHostCapabilities("pullImage", hosts.map((h) => h.id));
  const capability = useCapability("pullImage", { kind: "host", id: hostId });
  const parsed = parseImageReference(effectiveReference);
  const known = parsed ? images.some((i) => i.hostId === hostId && i.repository === parsed.repository && i.tag === parsed.tag) : false;
  const host = hosts.find((h) => h.id === hostId);
  const valid = !!parsed && host?.status === "online" && capability.available && (registryId === "auto" || !!selectedRegistry);

  // Busy only for the request; the pull itself continues as an action toast after the dialog closes.
  const pull = useBusyAction(async () => {
    const current = feedback.checkpoint();
    const id = await feedback.run(() => cmd.pull({ hostId, reference: effectiveReference, onProgress: (value) => { if (current()) setProgress({ owner: `${hostId}|${registryId}`, value }); } }, fail));
    if (id) change(false);
  });
  const submit = async () => {
    setTouched(true);
    if (!valid) return;
    await pull.run();
  };

  return (
    <Dialog open={open} onOpenChange={change}>
      <DialogContent {...feedback.bindings} className="max-h-[calc(100dvh-2rem)] overflow-y-auto sm:max-w-lg" data-surface="pull-dialog">
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
                    <SelectItem key={h.id} value={h.id} disabled={h.status !== "online" || !capabilities.get(h.id)}>
                      {h.status === "online" ? h.name : t("hostOffline", { name: h.name })}
                    </SelectItem>
                  ))}
                </SelectGroup>
              </SelectContent>
            </Select>
          </div>
          <div className="flex flex-col gap-2"><Label htmlFor="pull-registry">{t("registry")}</Label><Select value={registryId} onValueChange={setRegistryId}><SelectTrigger id="pull-registry"><SelectValue /></SelectTrigger><SelectContent><SelectItem value="auto">{t("automaticRegistry")}</SelectItem>{registries.data?.filter((r) => r.enabled).map((r) => <SelectItem key={r.id} value={r.id}>{r.url}</SelectItem>)}</SelectContent></Select></div>
          <TextField id="pull-ref" label={t("reference")} value={reference} onChange={setReference} mono placeholder="redis:7.4-alpine" description={known ? t("known") : t("referenceHint")} error={(touched || reference.length > 2) && !parsed ? t("referenceInvalid") : undefined} />
          <div className="flex flex-wrap items-center gap-1.5 text-xs">
            <span className="text-muted-foreground">{t("suggestions")}</span>
            {SUGGESTIONS.map((s) => (
              <Button key={s} type="button" size="sm" variant="outline" className="h-6 px-2 font-mono text-[11px]" onClick={() => setReference(s)}>
                {s}
              </Button>
            ))}
          </div>
          {import.meta.env.DEV && <div className="flex items-center gap-2">
            <Checkbox id="pull-fail" checked={fail} onCheckedChange={(v) => setFail(v === true)} />
            <Label htmlFor="pull-fail" className="text-sm font-normal text-muted-foreground">
              {t("fail")}
            </Label>
          </div>}
        </form>
        {progress?.owner === `${hostId}|${registryId}` && <p role="status" className="text-sm">{t(`progress.${progress.value.phase}`)}{progress.value.total ? ` · ${t("progressBytes", { current: progress.value.current ?? 0, total: progress.value.total })}` : ""}</p>}
        <DialogCommandFooter feedback={feedback}>
          <Button type="button" variant="outline" onClick={() => change(false)}>
            {t("cancel")}
          </Button>
          <BusyButton type="submit" form="pull-image" disabled={!valid} busy={pull.busy} icon={<Download data-icon="inline-start" />}>
            {t("submit")}
          </BusyButton>
        </DialogCommandFooter>
      </DialogContent>
    </Dialog>
  );
}
